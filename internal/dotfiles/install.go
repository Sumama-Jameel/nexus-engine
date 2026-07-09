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

package dotfiles

import (
	"context"
	"fmt"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
)

// InstallDeps holds dependencies required to install Chezmoi via the
// Orchestrator. The set is intentionally minimal — just what Install needs.
//
// Per Zero-Trust: ExecFn is required. We never shell out from the dotfiles
// package directly; every subprocess call goes through the caller-provided
// function (typically engine.SanitizeAndExecute).
type InstallDeps struct {
	PM     installer.PackageManager
	ExecFn ExecFunc
	State  *engine.StateTracker
	Audit  *engine.AuditLogger
	DryRun bool
}

// InstallResult captures the outcome of installing Chezmoi.
// Version is empty when PostInstall verification could not parse it
// (chezmoi is installed but `chezmoi --version` returned an unexpected
// shape — rare, but treated as a soft warning, not a hard error).
type InstallResult struct {
	Installed      bool   `json:"installed"`
	Version        string `json:"version,omitempty"`
	PackageManager string `json:"package_manager"`
}

// InstallChezmoi installs Chezmoi using the given package manager through
// the standard Orchestrator flow: PreFlight → RefreshIndex → Order →
// Execute → Verify → Record → Audit.
//
// On success, the Orchestrator records the install in ~/.nexus/state.json
// (as a managed package) and writes an audit entry. On failure, the typed
// installer.NexusError is wrapped and returned.
//
// For systems where Chezmoi is not in the OS package repository, this
// function will fail with the underlying package-manager error. Callers
// should surface that error directly to the user — adding a GitHub-releases
// fallback is out of scope for V7.
func InstallChezmoi(ctx context.Context, deps InstallDeps) (*InstallResult, error) {
	if deps.PM == nil {
		return nil, fmt.Errorf("dotfiles: InstallDeps.PM must not be nil")
	}
	if deps.ExecFn == nil {
		return nil, fmt.Errorf("dotfiles: InstallDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}

	// Use "dotfiles" as the profile name in audit/state entries so it's
	// distinguishable from user-driven profile installs.
	orch := installer.NewOrchestrator(deps.PM, deps.ExecFn, deps.State, deps.Audit, "dotfiles", deps.DryRun)

	if _, err := orch.Install(ctx, []string{"chezmoi"}); err != nil {
		return nil, fmt.Errorf("chezmoi install via %s failed: %w", deps.PM.Name(), err)
	}

	// Post-install verification: re-probe to confirm chezmoi is callable
	// AND to capture the installed version for the state record.
	report, verifyErr := Detect(ctx, deps.ExecFn)
	if verifyErr != nil {
		// Install succeeded but verification could not run. Surface a
		// clear warning but treat the install itself as successful —
		// the user can re-run `nexus dotfiles detect` later.
		return &InstallResult{
			Installed:      true,
			PackageManager: deps.PM.Name(),
		}, fmt.Errorf("chezmoi installed but post-install verification failed: %w", verifyErr)
	}

	if !report.Installed {
		// The package manager reported success but chezmoi is not on PATH.
		// This usually means a broken PATH or a non-standard install location.
		return &InstallResult{
			Installed:      false,
			PackageManager: deps.PM.Name(),
		}, fmt.Errorf("chezmoi installation reported success but binary is not on PATH")
	}

	// Record the dotfiles state. Best-effort: a state-record failure must
	// not turn a successful install into a user-facing error.
	if deps.State != nil {
		_ = deps.State.RecordDotfilesInstall(report.Version)
	}

	return &InstallResult{
		Installed:      true,
		Version:        report.Version,
		PackageManager: deps.PM.Name(),
	}, nil
}
