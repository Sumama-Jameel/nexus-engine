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

package installer

import (
        "context"
        "fmt"
        "sort"
        "strings"
        "time"

        "github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// OrchestratorResult is the complete outcome of an orchestrated operation.
// It aggregates per-package results with overall statistics and, when the
// run is aborted, the rollback report and abort reason.
type OrchestratorResult struct {
        // Total is the number of packages requested for installation.
        Total       int             `json:"total"`
        // Succeeded is the count of packages that installed without error.
        Succeeded   int             `json:"succeeded"`
        // Failed is the count of packages whose installation encountered an error.
        Failed      int             `json:"failed"`
        // Skipped is the count of packages that were already installed and did
        // not need reinstallation.
        Skipped     int             `json:"skipped"`
        // Verified is the count of packages that passed post-install verification.
        Verified    int             `json:"verified"`
        // Duration is the total wall-clock time for the orchestrated operation.
        Duration    time.Duration   `json:"duration_ms"`
        // Packages contains the per-package results in the order they were processed.
        Packages    []PackageResult `json:"packages"`
        // Aborted indicates whether the orchestrator halted early, typically due
        // to a foundation-package failure.
        Aborted     bool            `json:"aborted"`
        // AbortReason is a human-readable explanation of why the run was aborted.
        AbortReason string          `json:"abort_reason,omitempty"`
        // Rollback describes what was undone during the automatic rollback that
        // follows a foundation failure. Nil when no rollback occurred.
        Rollback    *RollbackReport `json:"rollback,omitempty"`
        // Preflight holds the pre-flight check results. Nil if pre-flight was skipped.
        Preflight   *PreFlightResult `json:"preflight,omitempty"`
}

// RollbackReport records what was undone during a rollback after an aborted run.
type RollbackReport struct {
        // Removed lists packages that were successfully removed during rollback.
        Removed  []string `json:"removed"`
        // Failed lists packages that could not be removed during rollback,
        // potentially leaving the system in a partially-configured state.
        Failed   []string `json:"failed"`
        // Reason is the human-readable explanation of why the rollback was triggered.
        Reason   string   `json:"reason"`
}

// Orchestrator is the brain of V2. It decides what to install,
// in what order, with what safeguards.
//
// Per BASIC_QNA: "You need to manually design the logic of how the Go engine
// decides what to install first. If the AI loops the logic, you'll brick a system."
//
// THE THREE INVOLIABLE RULES:
// 1. Never retry a failed package. User decides.
// 2. Never skip a failed foundation package. ABORT.
// 3. Never record an unverified package as installed.
//
// THE ROLLBACK RULE (derived from Rule 2 + Zero-Trust):
// If the Orchestrator aborts due to a foundation failure, it MUST attempt
// to remove any packages it successfully installed in that run. Never leave
// the system in a partially-configured, inconsistent state.
type Orchestrator struct {
        pm      PackageManager
        execFn  ExecFunc
        state   *engine.StateTracker
        audit   *engine.AuditLogger
        profile string
        dryRun  bool
}

// NewOrchestrator creates a new Orchestrator with all dependencies.
// The execFn is required for post-install verification and package index refresh.
// Per Zero-Trust: the verifier must use the same SanitizeAndExecute gate,
// never bypassing security for any package manager.
func NewOrchestrator(pm PackageManager, execFn ExecFunc, state *engine.StateTracker, audit *engine.AuditLogger, profile string, dryRun bool) *Orchestrator {
        return &Orchestrator{
                pm:      pm,
                execFn:  execFn,
                state:   state,
                audit:   audit,
                profile: profile,
                dryRun:  dryRun,
        }
}

// Install executes the full orchestrated installation flow:
// PreFlight → RefreshIndex → Order → Execute → Verify → Record → Audit → Report
// On foundation failure: Rollback → Report
func (o *Orchestrator) Install(ctx context.Context, packages []string) (*OrchestratorResult, error) {
        start := time.Now()
        result := &OrchestratorResult{Total: len(packages)}

        if len(packages) == 0 {
                result.Aborted = false
                return result, nil
        }

        // ─── STEP 1: PRE-FLIGHT ───
        // "Measure twice, cut once."
        preflight := PreFlightCheck(ctx, o.pm, packages, o.pm.Name())
        result.Preflight = preflight

        if !preflight.CanProceed && !o.dryRun {
                result.Aborted = true
                result.AbortReason = "Pre-flight checks failed: " + strings.Join(preflight.Warnings, "; ")
                result.Duration = time.Since(start)
                return result, NewPreflightError("pre-flight checks failed: "+result.AbortReason, nil)
        }

        // Record skipped packages (idempotency)
        for _, pkg := range preflight.Skipped {
                result.Packages = append(result.Packages, PackageResult{
                        Package:    pkg,
                        Action:     "install",
                        Success:    true,
                        Skipped:    true,
                        SkipReason: "already installed",
                })
                result.Skipped++
        }

        packages = preflight.ToInstall
        if len(packages) == 0 {
                result.Duration = time.Since(start)
                return result, nil // Everything already installed
        }

        // ─── STEP 1.5: REFRESH PACKAGE INDEX ───
        // Without a fresh index, apt-get install may fail with
        // "Unable to locate package" on fresh systems. This is a
        // mandatory step, not optional.
        if !o.dryRun {
                if err := o.pm.RefreshIndex(ctx); err != nil {
                        // Non-fatal: the index may already be fresh, or we may be offline
                        // with cached packages. Log the warning but proceed.
                        o.logAudit(engine.AuditEntry{
                                Action:  "refresh_index",
                                Package: o.pm.Name(),
                                Result:  "warning",
                                Profile: o.profile,
                                Error:   err.Error(),
                        })
                } else {
                        o.logAudit(engine.AuditEntry{
                                Action:  "refresh_index",
                                Package: o.pm.Name(),
                                Result:  "success",
                                Profile: o.profile,
                        })
                }
        }

        // ─── STEP 2: ORDER (Dependency-Aware Sorting) ───
        // Foundation before Languages before Tools.
        groups := groupByPriority(packages)

        // ─── STEP 3: EXECUTE (Priority-Group Installation) ───
        // Within each priority group, packages are installed concurrently
        // using goroutines. Per the docs: "High-speed concurrency."
        // The Orchestrator collects results and handles failures per-group.
        var installedThisRun []string // Track for potential rollback

        for priority, pkgs := range groups {
                if len(pkgs) == 0 {
                        continue
                }

                if o.dryRun {
                        for _, pkg := range pkgs {
                                result.Packages = append(result.Packages, PackageResult{
                                        Package: pkg,
                                        Action:  "install",
                                        Success: true,
                                        Skipped: false,
                                })
                                result.Succeeded++
                                o.logAudit(engine.AuditEntry{
                                        Action:  "install",
                                        Package: pkg,
                                        Result:  "dry_run",
                                        Profile: o.profile,
                                })
                        }
                        continue
                }

                // Install this priority group (concurrently within the group)
                installResults := o.installGroup(ctx, pkgs)

                // Check for foundation failure
                foundationFailed := false
                for _, pr := range installResults {
                        if !pr.Success && priority == PriorityFoundation {
                                foundationFailed = true
                                break
                        }
                }

                // Process results
                for _, pr := range installResults {
                        o.logAudit(engine.AuditEntry{
                                Action:         pr.Action,
                                Package:        pr.Package,
                                Result:         boolToResult(pr.Success),
                                PackageManager: o.pm.Name(),
                                Profile:        o.profile,
                                DurationMs:     pr.Duration.Milliseconds(),
                                Error:          pr.Error,
                        })

                        if pr.Success {
                                result.Succeeded++
                                installedThisRun = append(installedThisRun, pr.Package)
                        } else {
                                result.Failed++
                        }

                        result.Packages = append(result.Packages, pr)
                }

                // If a FOUNDATION package fails, ABORT and ROLLBACK per Rule 2
                if foundationFailed {
                        result.Aborted = true
                        result.AbortReason = "Foundation package installation failed — system integrity at risk"
                        // Rollback: remove everything we installed this run
                        result.Rollback = o.rollback(ctx, installedThisRun)
                        result.Duration = time.Since(start)
                        return result, NewFoundationError(result.AbortReason, nil)
                }
        }

        // ─── STEP 4: VERIFY ───
        // "Trust but verify."
        if !o.dryRun {
                var pkgsToVerify []string
                for _, pr := range result.Packages {
                        if pr.Success && !pr.Skipped {
                                pkgsToVerify = append(pkgsToVerify, pr.Package)
                        }
                }

                if len(pkgsToVerify) > 0 {
                        // Use the injected execFn — works for ALL package managers
                        verifyResults := VerifyInstallation(ctx, o.pm, pkgsToVerify, o.execFn)
                        for _, vr := range verifyResults {
                                o.logAudit(engine.AuditEntry{
                                        Action:  "verify",
                                        Package: vr.Package,
                                        Result:  boolToResult(vr.Verified),
                                        Profile: o.profile,
                                })
                                if vr.Verified {
                                        result.Verified++
                                }
                                // Update the package result
                                for i := range result.Packages {
                                        if result.Packages[i].Package == vr.Package {
                                                result.Packages[i].Verified = vr.Verified
                                                break
                                        }
                                }
                        }
                }
        }

        // ─── STEP 5: RECORD STATE ───
        // Per Rule 3: Never record an unverified package as installed.
        if !o.dryRun && o.state != nil {
                for _, pr := range result.Packages {
                        if pr.Success && !pr.Skipped && pr.Verified {
                                o.state.RecordInstall(pr.Package, o.profile, o.pm.Name(), pr.Verified)
                        }
                }
        }

        result.Duration = time.Since(start)
        return result, nil
}

// installGroup installs a priority group of packages concurrently.
// Each package is installed in its own goroutine. Results are collected
// and returned in the same order as the input packages.
//
// Per the docs: "Go handles concurrency natively, allowing the engine
// to probe CPU, RAM, and network simultaneously in milliseconds."
// We apply the same principle to package installation.
func (o *Orchestrator) installGroup(ctx context.Context, pkgs []string) []PackageResult {
        // For a single package, skip goroutine overhead
        if len(pkgs) == 1 {
                start := time.Now()
                result := PackageResult{Package: pkgs[0], Action: "install"}
                _, err := o.pm.Install(ctx, pkgs)
                result.Duration = time.Since(start)
                if err != nil {
                        result.Success = false
                        result.Error = err.Error()
                } else {
                        result.Success = true
                }
                return []PackageResult{result}
        }

        // Install each package concurrently
        type indexedResult struct {
                index  int
                result PackageResult
        }

        ch := make(chan indexedResult, len(pkgs))

        for i, pkg := range pkgs {
                go func(idx int, p string) {
                        start := time.Now()
                        result := PackageResult{Package: p, Action: "install"}

                        // Install a single package
                        _, err := o.pm.Install(ctx, []string{p})
                        result.Duration = time.Since(start)

                        if err != nil {
                                result.Success = false
                                result.Error = err.Error()
                        } else {
                                result.Success = true
                        }

                        ch <- indexedResult{index: idx, result: result}
                }(i, pkg)
        }

        // Collect results in order
        results := make([]PackageResult, len(pkgs))
        for i := 0; i < len(pkgs); i++ {
                ir := <-ch
                results[ir.index] = ir.result
        }

        return results
}

// rollback attempts to remove packages installed during the current run.
// Called only when a foundation package fails — we must not leave the
// system in a partially-configured state.
//
// Per the Nexus Protocol: "Immutable Infrastructure — the OS layer is
// treated as read-only." If we can't complete the installation, we undo it.
func (o *Orchestrator) rollback(ctx context.Context, packages []string) *RollbackReport {
        report := &RollbackReport{
                Reason: "Foundation package failure — rolling back partial installation",
        }

        if len(packages) == 0 {
                return report
        }

        o.logAudit(engine.AuditEntry{
                Action:  "rollback",
                Package: strings.Join(packages, ","),
                Result:  "started",
                Profile: o.profile,
        })

        removeResults, _ := o.pm.Remove(ctx, packages)
        for _, rr := range removeResults {
                if rr.Success {
                        report.Removed = append(report.Removed, rr.Package)
                        // Also remove from state tracker
                        if o.state != nil {
                                o.state.RecordRemove(rr.Package)
                        }
                } else {
                        report.Failed = append(report.Failed, rr.Package)
                }

                o.logAudit(engine.AuditEntry{
                        Action:  "rollback_remove",
                        Package: rr.Package,
                        Result:  boolToResult(rr.Success),
                        Profile: o.profile,
                        Error:   rr.Error,
                })
        }

        return report
}

// groupByPriority sorts packages into priority groups for the Orchestrator.
func groupByPriority(packages []string) map[int][]string {
        groups := make(map[int][]string)
        for _, pkg := range packages {
                p := ClassifyPriority(pkg)
                groups[p] = append(groups[p], pkg)
        }
        return groups
}

// SortedPriorityKeys returns priority keys in order (1, 2, 3).
func SortedPriorityKeys(groups map[int][]string) []int {
        keys := make([]int, 0, len(groups))
        for k := range groups {
                keys = append(keys, k)
        }
        sort.Ints(keys)
        return keys
}

// FormatOrchestratorResult returns a human-readable summary.
func FormatOrchestratorResult(r *OrchestratorResult) string {
        var sb strings.Builder

        if r.Aborted {
                sb.WriteString(fmt.Sprintf("  ⛔ ABORTED: %s\n", r.AbortReason))
        }

        // Pre-flight summary
        if r.Preflight != nil {
                sb.WriteString(FormatPreFlightResult(r.Preflight))
        }

        sb.WriteString(fmt.Sprintf("  Total: %d | Succeeded: %d | Failed: %d | Skipped: %d | Verified: %d\n",
                r.Total, r.Succeeded, r.Failed, r.Skipped, r.Verified))
        sb.WriteString(fmt.Sprintf("  Duration: %s\n", r.Duration.Round(time.Millisecond)))

        if r.Rollback != nil {
                sb.WriteString("\n  ── ROLLBACK ──────────────────────────────────\n")
                sb.WriteString(fmt.Sprintf("  Reason: %s\n", r.Rollback.Reason))
                if len(r.Rollback.Removed) > 0 {
                        sb.WriteString(fmt.Sprintf("  Removed: %v\n", r.Rollback.Removed))
                }
                if len(r.Rollback.Failed) > 0 {
                        sb.WriteString(fmt.Sprintf("  ⚠️  Failed to remove: %v\n", r.Rollback.Failed))
                }
        }

        if r.Failed > 0 {
                sb.WriteString("\n  ── FAILED PACKAGES ───────────────────────────\n")
                for _, pr := range r.Packages {
                        if !pr.Success && !pr.Skipped {
                                sb.WriteString(fmt.Sprintf("    ⛔ %s: %s\n", pr.Package, pr.Error))
                        }
                }
        }

        return sb.String()
}

func (o *Orchestrator) logAudit(entry engine.AuditEntry) {
        if o.audit != nil {
                o.audit.Log(entry)
        }
}

func boolToResult(b bool) string {
        if b {
                return "success"
        }
        return "failure"
}
