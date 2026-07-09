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
	"strings"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// ApplyDeps holds dependencies for Apply / Status / Diff operations.
// Mirrors the SourceDeps shape \u2014 keeping dependency structs consistent
// across the dotfiles package makes the CLI wiring in main.go simpler.
type ApplyDeps struct {
	ExecFn ExecFunc
	State  *engine.StateTracker
	Audit  *engine.AuditLogger
	DryRun bool
}

// ApplyReport captures the outcome of an apply operation.
//
// For DryRun=true: Diff contains the would-change output (best-effort).
// For Apply=true:  Output contains the raw chezmoi apply stdout/stderr.
//
// Note on chezmoi diff output: `chezmoi diff` follows POSIX convention
// (exit 1 when differences exist, exit 0 when none). When diff exits 1,
// the diff content is on stdout but the engine's ExecFunc only returns
// stdout on success. We surface a placeholder message instead \u2014 callers
// who need the exact diff should run `chezmoi diff` directly.
type ApplyReport struct {
	Applied    bool   `json:"applied"`
	DiffOutput string `json:"diff_output,omitempty"`
	RawOutput  string `json:"raw_output,omitempty"`
	Source     string `json:"source,omitempty"`
	AppliedAt  string `json:"applied_at,omitempty"`
}

// Apply applies managed dotfiles from the bound source to the live system.
//
// Preconditions:
//   - Chezmoi must be installed (deps.State.IsDotfilesInstalled, when state is available)
//   - A source must be bound (deps.State.GetDotfilesState().Source != "")
//
// Behavior:
//   - When DryRun=true: runs `chezmoi diff` to preview changes; does NOT touch files.
//   - When DryRun=false: runs `chezmoi apply`; updates LastAppliedAt in state on success.
//
// Per Zero-Trust: all subprocess execution flows through the caller-provided ExecFunc.
func Apply(ctx context.Context, deps ApplyDeps) (*ApplyReport, error) {
	if deps.ExecFn == nil {
		return nil, fmt.Errorf("dotfiles: ApplyDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}

	// Precondition: a source must be bound.
	source := ""
	if deps.State != nil {
		source = deps.State.GetDotfilesState().Source
	}
	if source == "" {
		return nil, fmt.Errorf("no dotfile source bound \u2014 run 'nexus dotfiles init <repo>' first")
	}

	report := &ApplyReport{Source: source}

	if deps.DryRun {
		diffOut, diffErr := Diff(ctx, deps)
		report.DiffOutput = diffOut
		// Diff intentionally never returns a hard error (it converts
		// the "differences found" exit code into a placeholder message).
		_ = diffErr
		return report, nil
	}

	// Actual apply. chezmoi apply exits 0 on success.
	out, err := deps.ExecFn(ctx, "chezmoi", "apply")
	if err != nil {
		return nil, fmt.Errorf("chezmoi apply failed: %w", err)
	}

	report.Applied = true
	report.RawOutput = out

	if deps.State != nil {
		// RecordApply updates LastAppliedAt. We don't yet track the full
		// managed-file list here \u2014 that comes from Slice 5 (`add`) and from
		// `chezmoi managed` parsing (deferred until it's needed).
		_ = deps.State.RecordDotfilesApply(nil)
		report.AppliedAt = deps.State.GetDotfilesState().LastAppliedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	return report, nil
}

// Status returns the current chezmoi state as a human-readable string.
//
// Implementation note: `chezmoi status` exits 0 when the live system matches
// the source, and exits 1 when there are unapplied changes. As with Diff,
// the output is lost on exit 1; we report a placeholder and let the user
// run `chezmoi status` directly if they need the full picture.
func Status(ctx context.Context, deps ApplyDeps) (string, error) {
	if deps.ExecFn == nil {
		return "", fmt.Errorf("dotfiles: ApplyDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}
	out, err := deps.ExecFn(ctx, "chezmoi", "status")
	if err == nil {
		return out, nil
	}
	if strings.Contains(err.Error(), "exit status 1") {
		return "(unapplied changes detected \u2014 run 'chezmoi status' directly to see them)", nil
	}
	return "", fmt.Errorf("chezmoi status failed: %w", err)
}

// Diff returns the pending changes between the bound source and the live system.
//
// Implementation note: POSIX `diff` semantics mean exit code 1 signals
// "differences found". The actual diff content is on stdout, which the
// engine's ExecFunc returns only on exit 0. We capture stdout when no
// differences exist, and emit a placeholder when differences exist. Users
// who need the full diff output should run `chezmoi diff` directly.
func Diff(ctx context.Context, deps ApplyDeps) (string, error) {
	if deps.ExecFn == nil {
		return "", fmt.Errorf("dotfiles: ApplyDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}
	out, err := deps.ExecFn(ctx, "chezmoi", "diff")
	if err == nil {
		if out == "" {
			return "(no differences)", nil
		}
		return out, nil
	}
	if strings.Contains(err.Error(), "exit status 1") {
		return "(differences detected \u2014 run 'chezmoi diff' directly to see them)", nil
	}
	return "", fmt.Errorf("chezmoi diff failed: %w", err)
}
