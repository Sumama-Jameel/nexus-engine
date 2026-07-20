// Copyright 2024-2026 Nexus Protocol Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mode

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/engineutil"
)

// ExecFunc is the Zero-Trust execution gate. Type alias so it's the
// same type as installer.ExecFunc — prevents cross-package type errors.
type ExecFunc = func(ctx context.Context, command string, args ...string) (string, error)

// ApplyDeps holds the dependencies for Apply / Rollback. Following the
// dotfiles.SyncDeps pattern: callbacks (not direct imports) for the
// profile and dotfiles operations keep internal/mode free of cross-package
// coupling and let main.go act as the composition root.
type ApplyDeps struct {
	// ExecFn is the Zero-Trust command gate. Required.
	ExecFn ExecFunc
	// State persists ActiveMode / LastMode* to ~/.nexus/state.json. Optional.
	State *engine.StateTracker
	// Audit writes MODE_APPLY* entries to ~/.nexus/audit.log. Optional.
	Audit *engine.AuditLogger
	// GOOS is the running OS: "linux", "windows", or "darwin". Required
	// for picking the right service manager / power tool.
	GOOS string
	// ApplyProfile is wired in main.go to installer.NewOrchestrator(...).Install.
	// Required. Implementations must honor the dryRun flag.
	ApplyProfile func(ctx context.Context, profileName string, dryRun bool) error
	// BindDotfiles is wired in main.go to dotfiles.BindSource. Optional;
	// when nil, modes with dotfiles_source fail with a clear error.
	BindDotfiles func(ctx context.Context, source string) error
}

// ApplyOpts controls a single apply invocation.
type ApplyOpts struct {
	// DryRun prints the plan without changing anything.
	DryRun bool
	// Yes skips the interactive confirmation prompt. The CLI uses this
	// when the user has already confirmed via a flag; the dashboard uses
	// it because the modal dialog IS the confirmation.
	Yes bool
	// AllowUnlistedServices lifts the service allowlist. Every unlisted
	// action is audit-logged with MODE_APPLY_UNLISTED_SERVICE.
	AllowUnlistedServices bool
}

// Step records one phase of the apply pipeline. Used in both DryRun plan
// output and live ApplyReport so the operator can see exactly what the
// engine did (or would have done).
type Step struct {
	Name    string `json:"name"`             // profile | dotfiles | stop_service | start_service | os_tweaks
	Detail  string `json:"detail,omitempty"` // e.g., profile name, service name
	Outcome string `json:"outcome"`          // success | failure | skipped
	Error   string `json:"error,omitempty"`
}

// ApplyReport is the structured outcome of Apply. JSON-serializable so
// the dashboard can consume it directly via `nexus mode apply --json`.
type ApplyReport struct {
	Mode        string    `json:"mode"`
	Previous    string    `json:"previous,omitempty"`
	DryRun      bool      `json:"dry_run,omitempty"`
	Plan        []Step    `json:"plan,omitempty"`
	Steps       []Step    `json:"steps"`
	Outcome     string    `json:"outcome"` // success | failure
	Error       string    `json:"error,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// Apply atomically switches the system to the named mode.
//
// The pipeline (ADR 010 § "Apply pipeline"):
//  1. Validate the mode YAML (already done by Resolve)
//  2. Validate service names against the allowlist
//  3. Snapshot current mode into Previous
//  4. Apply the target profile (callback)
//  5. Re-bind dotfiles if the mode specifies a source (callback)
//  6. Stop listed services
//  7. Start listed services
//  8. Apply OS tweaks
//  9. Persist ActiveMode + LastMode* to state.json
//  10. Audit MODE_APPLY
//
// On any failure: no auto-rollback. The previous mode stays Active until
// the operator runs `nexus mode rollback` (or another apply). This is
// deliberate: auto-rollback can cascade if rollback itself fails.
//
// Dry-run short-circuits at step 3 with a populated Plan and no side
// effects — useful for `nexus mode apply gamer --dry-run` from the CLI
// and for the dashboard's pre-confirm preview.
func Apply(ctx context.Context, deps ApplyDeps, name string, opts ApplyOpts) (*ApplyReport, error) {
	if deps.ExecFn == nil {
		return nil, errors.New("mode: ApplyDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}
	if deps.ApplyProfile == nil {
		return nil, errors.New("mode: ApplyDeps.ApplyProfile must not be nil")
	}
	if deps.GOOS == "" {
		return nil, errors.New("mode: ApplyDeps.GOOS must not be empty")
	}

	m, err := Resolve(name)
	if err != nil {
		return nil, err
	}

	report := &ApplyReport{
		Mode:      m.Name,
		StartedAt: time.Now().UTC(),
	}

	// Snapshot current mode so we can record where we came from.
	if deps.State != nil {
		report.Previous = deps.State.GetActiveMode()
	}

	if opts.DryRun {
		report.DryRun = true
		report.Plan = planSteps(m, deps.GOOS, opts.AllowUnlistedServices, deps.Audit)
		report.Outcome = "success"
		report.CompletedAt = time.Now().UTC()
		return report, nil
	}

	// Step 2: allowlist. Both lists share the same validation path.
	if err := validateServices(m.StopServices, deps.GOOS, opts.AllowUnlistedServices, deps.Audit, m.Name); err != nil {
		return fail(report, deps.Audit, m.Name, "stop_services", err)
	}
	if err := validateServices(m.StartServices, deps.GOOS, opts.AllowUnlistedServices, deps.Audit, m.Name); err != nil {
		return fail(report, deps.Audit, m.Name, "start_services", err)
	}

	// Step 4: profile.
	if err := deps.ApplyProfile(ctx, m.Profile, false); err != nil {
		report.Steps = append(report.Steps, Step{Name: "profile", Detail: m.Profile, Outcome: "failure", Error: err.Error()})
		return fail(report, deps.Audit, m.Name, "profile", err)
	}
	report.Steps = append(report.Steps, Step{Name: "profile", Detail: m.Profile, Outcome: "success"})

	// Step 5: dotfiles bind (optional).
	if m.DotfilesSource != "" {
		if deps.BindDotfiles == nil {
			err := errors.New("dotfiles_source set but ApplyDeps.BindDotfiles is nil")
			report.Steps = append(report.Steps, Step{Name: "dotfiles", Detail: m.DotfilesSource, Outcome: "failure", Error: err.Error()})
			return fail(report, deps.Audit, m.Name, "dotfiles", err)
		}
		if err := deps.BindDotfiles(ctx, m.DotfilesSource); err != nil {
			report.Steps = append(report.Steps, Step{Name: "dotfiles", Detail: m.DotfilesSource, Outcome: "failure", Error: err.Error()})
			return fail(report, deps.Audit, m.Name, "dotfiles", err)
		}
		report.Steps = append(report.Steps, Step{Name: "dotfiles", Detail: m.DotfilesSource, Outcome: "success"})
	} else {
		report.Steps = append(report.Steps, Step{Name: "dotfiles", Outcome: "skipped"})
	}

	// Steps 6 + 7: services. We do stop-then-start because some mode
	// transitions need a service cycle (e.g., docker → podman swap).
	for _, svc := range m.StopServices {
		if err := toggleService(ctx, deps.ExecFn, deps.GOOS, svc, "stop"); err != nil {
			report.Steps = append(report.Steps, Step{Name: "stop_service", Detail: svc, Outcome: "failure", Error: err.Error()})
			return fail(report, deps.Audit, m.Name, "stop_service "+svc, err)
		}
		report.Steps = append(report.Steps, Step{Name: "stop_service", Detail: svc, Outcome: "success"})
	}
	for _, svc := range m.StartServices {
		if err := toggleService(ctx, deps.ExecFn, deps.GOOS, svc, "start"); err != nil {
			report.Steps = append(report.Steps, Step{Name: "start_service", Detail: svc, Outcome: "failure", Error: err.Error()})
			return fail(report, deps.Audit, m.Name, "start_service "+svc, err)
		}
		report.Steps = append(report.Steps, Step{Name: "start_service", Detail: svc, Outcome: "success"})
	}

	// Step 8: OS tweaks.
	if err := applyOSTweaks(ctx, deps.ExecFn, deps.GOOS, m.OSTweaks); err != nil {
		report.Steps = append(report.Steps, Step{Name: "os_tweaks", Outcome: "failure", Error: err.Error()})
		return fail(report, deps.Audit, m.Name, "os_tweaks", err)
	}
	report.Steps = append(report.Steps, Step{Name: "os_tweaks", Outcome: "success"})

	// Step 9: state.
	if deps.State != nil {
		if err := deps.State.RecordModeApply(report.Previous, m.Name); err != nil {
			report.Steps = append(report.Steps, Step{Name: "state", Outcome: "failure", Error: err.Error()})
			return fail(report, deps.Audit, m.Name, "state", err)
		}
	}

	report.Outcome = "success"
	report.CompletedAt = time.Now().UTC()
	logAudit(deps.Audit, "MODE_APPLY", "success", fmt.Sprintf("applied %q (from %q)", m.Name, report.Previous))
	return report, nil
}

// Rollback re-applies the mode that was active immediately before the
// current one. It is the operator's recovery path when an apply fails
// partway and the live system is in an inconsistent state.
//
// No-op (with a clear error) when no previous mode is recorded.
func Rollback(ctx context.Context, deps ApplyDeps, opts ApplyOpts) (*ApplyReport, error) {
	if deps.State == nil {
		return nil, errors.New("rollback requires ApplyDeps.State")
	}
	prev := deps.State.GetModeState().Previous
	if prev == "" {
		return nil, errors.New("no previous mode recorded — nothing to roll back to")
	}
	report, err := Apply(ctx, deps, prev, opts)
	if err != nil {
		logAudit(deps.Audit, "MODE_ROLLBACK", "failure", fmt.Sprintf("target=%s: %v", prev, err))
		return report, err
	}
	logAudit(deps.Audit, "MODE_ROLLBACK", "success", "reverted to "+prev)
	return report, nil
}

// fail centralizes the failure bookkeeping so each error branch in Apply
// is one line. It always returns a non-nil error — callers should `return`
// the result of this directly.
func fail(report *ApplyReport, audit *engine.AuditLogger, modeName, phase string, cause error) (*ApplyReport, error) {
	report.Outcome = "failure"
	report.Error = phase + ": " + cause.Error()
	report.CompletedAt = time.Now().UTC()
	logAudit(audit, "MODE_APPLY_FAILED", "failure", fmt.Sprintf("%s [%s]: %v", modeName, phase, cause))
	return report, fmt.Errorf("mode apply %q failed at %s: %w", modeName, phase, cause)
}

// validateServices enforces the allowlist policy from ADR 010 §
// "Service allowlist — Option C". When a service is unlisted and the
// operator opted in via --allow-unlisted-services, we DO NOT fail —
// instead we emit a loud WARNING audit entry so the action is traceable.
func validateServices(names []string, goos string, allowUnlisted bool, audit *engine.AuditLogger, modeName string) error {
	for _, svc := range names {
		if IsServiceAllowed(svc, goos) {
			continue
		}
		if allowUnlisted {
			logAudit(audit, "MODE_APPLY_UNLISTED_SERVICE", "warning",
				fmt.Sprintf("%s: unlisted service %q on %s", modeName, svc, goos))
			continue
		}
		return &ErrServiceNotAllowed{Service: svc, GOOS: goos}
	}
	return nil
}

// toggleService shells out to the platform-appropriate service manager.
// All execution flows through the Zero-Trust gate (deps.ExecFn).
//
// Linux:   systemctl <action> <name>
// Windows: sc <action> <name>  (sc start/stop; "stop" → "stop", "start" → "start")
//
// The function does not verify the service exists beforehand — systemctl
// and sc both return non-zero with a clear error if the service is
// missing, which we surface verbatim.
func toggleService(ctx context.Context, execFn ExecFunc, goos, name, action string) error {
	switch goos {
	case "linux":
		_, err := execFn(ctx, "systemctl", action, name)
		return err
	case "windows":
		_, err := execFn(ctx, "sc", action, name)
		return err
	default:
		return fmt.Errorf("service toggling not supported on %q", goos)
	}
}

// applyOSTweaks applies the OS-specific tweaks. Both arms are no-ops if
// the relevant field is empty, so a portable mode YAML works on either
// OS without surprises.
//
// Linux:   cpupower frequency-set -g <governor>
// Windows: powercfg /setactive <plan>
//
// We deliberately do not check whether the tweak tool is installed: the
// engine's CommandTimeoutSec + non-zero exit surfacing is enough to make
// a missing cpupower visible without masking real errors.
func applyOSTweaks(ctx context.Context, execFn ExecFunc, goos string, t OSTweaks) error {
	switch goos {
	case "linux":
		if t.Linux.CPUGovernor != "" {
			if _, err := execFn(ctx, "cpupower", "frequency-set", "-g", t.Linux.CPUGovernor); err != nil {
				return fmt.Errorf("cpupower frequency-set failed: %w", err)
			}
		}
		return nil
	case "windows":
		if t.Windows.PowerPlan != "" {
			guid, err := resolvePowerPlan(t.Windows.PowerPlan)
			if err != nil {
				return err
			}
			if _, err := execFn(ctx, "powercfg", "/setactive", guid); err != nil {
				return fmt.Errorf("powercfg /setactive failed: %w", err)
			}
		}
		return nil
	default:
		// macOS / unknown: tweaks are no-ops today. Fail silently to keep
		// a portable mode YAML usable everywhere.
		return nil
	}
}

// resolvePowerPlan maps the friendly alias to the canonical Windows GUID.
// Aliases match the three power plans every Windows install ships with;
// we do not enumerate third-party plans here — powercfg can list those
// via `powercfg /list` if a user needs something exotic.
func resolvePowerPlan(alias string) (string, error) {
	switch strings.ToLower(alias) {
	case "balanced":
		return "381b4222-f694-41f0-9685-ff5bb260df2e", nil
	case "high_performance", "high-performance", "performance":
		return "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c", nil
	case "power_saver", "power-saver", "saver":
		return "a1841308-3541-4fab-bc81-f71556f20b4a", nil
	default:
		// Allow passing a raw GUID for advanced operators.
		if strings.Count(alias, "-") == 4 && len(alias) == 36 {
			return alias, nil
		}
		return "", fmt.Errorf("unknown power plan alias %q (use balanced, high_performance, power_saver, or a GUID)", alias)
	}
}

// planSteps produces the would-execute list for DryRun. The Step slice is
// deterministic (services are sorted) so the CLI and dashboard can
// diff two dry-runs for the same mode reliably.
func planSteps(m *Mode, goos string, allowUnlisted bool, audit *engine.AuditLogger) []Step {
	steps := []Step{
		{Name: "profile", Detail: m.Profile, Outcome: "success"},
	}
	if m.DotfilesSource != "" {
		steps = append(steps, Step{Name: "dotfiles", Detail: m.DotfilesSource, Outcome: "success"})
	} else {
		steps = append(steps, Step{Name: "dotfiles", Outcome: "skipped"})
	}
	stopSorted := append([]string(nil), m.StopServices...)
	startSorted := append([]string(nil), m.StartServices...)
	sort.Strings(stopSorted)
	sort.Strings(startSorted)
	for _, svc := range stopSorted {
		outcome := "success"
		if !IsServiceAllowed(svc, goos) {
			if allowUnlisted {
				outcome = "success (unlisted, audit-logged)"
			} else {
				outcome = "blocked (unlisted)"
			}
		}
		steps = append(steps, Step{Name: "stop_service", Detail: svc, Outcome: outcome})
	}
	for _, svc := range startSorted {
		outcome := "success"
		if !IsServiceAllowed(svc, goos) {
			if allowUnlisted {
				outcome = "success (unlisted, audit-logged)"
			} else {
				outcome = "blocked (unlisted)"
			}
		}
		steps = append(steps, Step{Name: "start_service", Detail: svc, Outcome: outcome})
	}
	switch goos {
	case "linux":
		if m.OSTweaks.Linux.CPUGovernor != "" {
			steps = append(steps, Step{Name: "os_tweaks", Detail: "cpu_governor=" + m.OSTweaks.Linux.CPUGovernor, Outcome: "success"})
		} else {
			steps = append(steps, Step{Name: "os_tweaks", Outcome: "skipped"})
		}
	case "windows":
		if m.OSTweaks.Windows.PowerPlan != "" {
			steps = append(steps, Step{Name: "os_tweaks", Detail: "power_plan=" + m.OSTweaks.Windows.PowerPlan, Outcome: "success"})
		} else {
			steps = append(steps, Step{Name: "os_tweaks", Outcome: "skipped"})
		}
	default:
		steps = append(steps, Step{Name: "os_tweaks", Outcome: "skipped"})
	}
	return steps
}

// logAudit writes an audit entry when an AuditLogger is provided.
// Failures are best-effort and do not propagate. Same shape as the
// helper in internal/dotfiles/sync.go so the audit format is uniform.
func logAudit(a *engine.AuditLogger, action, result, detail string) {
	engineutil.LogAudit(a, action, result, detail)
}
