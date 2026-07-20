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
	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
)

// ProfileDeps is the superset of dependencies needed to apply a profile's
// dotfile section. It bundles SourceDeps, ApplyDeps, and AddDeps because
// a profile-driven flow touches all three operations in sequence.
type ProfileDeps struct {
	ExecFn ExecFunc
	State  *engine.StateTracker
	Audit  *engine.AuditLogger
}

// ProfileApplyReport captures what ApplyFromProfile did (or attempted).
//
// All fields are populated best-effort. Even when one step fails, the
// remaining steps are attempted \u2014 callers can decide which failures
// matter for their use case (e.g., 'nexus init' treats dotfile failures
// as warnings, not fatal errors).
type ProfileApplyReport struct {
	SourceBound  bool     `json:"source_bound"`
	Applied      bool     `json:"applied"`
	Pushed       bool     `json:"pushed,omitempty"` // V8: true if sync_on_apply triggered a push
	AddedPaths   []string `json:"added_paths,omitempty"`
	SkippedPaths []string `json:"skipped_paths,omitempty"` // sensitive paths that need --force
	Warnings     []string `json:"warnings,omitempty"`
}

// ApplyFromProfile applies a profile's dotfiles section end-to-end:
//
//  1. If profile.Dotfiles.Source is set: bind the source repo.
//  2. If profile.Dotfiles.SyncOnApply is true AND a source was bound: run
//     a full pull + apply + push sync (V8). When sync runs, the separate
//     apply_on_init step below is skipped because Sync already applies.
//  3. If profile.Dotfiles.ApplyOnInit is true (and sync_on_apply is false):
//     apply dotfiles from the bound source.
//  4. For each path in profile.Dotfiles.ManagedPaths: track it with 'chezmoi add'.
//     Sensitive paths are skipped (not failed) — the user must explicitly
//     run 'nexus dotfiles add <path> --force' for those.
//
// The function NEVER returns an error for individual step failures. All
// failures are accumulated into Warnings so callers can surface them
// without aborting the parent operation (e.g., 'nexus init').
//
// Per V7 Slice 6 plan: this is the bridge between the manifest package
// (declarative intent) and the dotfiles package (imperative operations).
func ApplyFromProfile(ctx context.Context, profile *manifest.NexusProfile, deps ProfileDeps) *ProfileApplyReport {
	report := &ProfileApplyReport{}

	if profile == nil || profile.Dotfiles == nil {
		// No dotfiles section in this profile \u2014 nothing to do.
		return report
	}

	if deps.ExecFn == nil {
		report.Warnings = append(report.Warnings, "ExecFunc is nil; cannot apply dotfiles from profile")
		return report
	}

	dot := profile.Dotfiles

	// Step 1: Bind the source if specified.
	if dot.Source != "" {
		sourceDeps := SourceDeps{
			ExecFn: deps.ExecFn,
			State:  deps.State,
			Audit:  deps.Audit,
		}
		if err := BindSource(ctx, dot.Source, sourceDeps); err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("bind source %q: %v", dot.Source, err))
			// Don't proceed to apply/add if bind failed.
			return report
		}
		report.SourceBound = true
	}

	// Step 2: Sync if requested (V8). Full pull + apply + push cycle.
	// When sync succeeds, it sets report.Applied and report.Pushed. The
	// separate apply_on_init step below is skipped because Sync already
	// applies the pulled state.
	syncRan := false
	if dot.SyncOnApply && report.SourceBound {
		syncDeps := SyncDeps{
			ExecFn: deps.ExecFn,
			State:  deps.State,
			Audit:  deps.Audit,
		}
		syncReport, err := Sync(ctx, syncDeps, "", false)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("sync: %v", err))
		} else {
			report.Applied = syncReport.Applied
			report.Pushed = syncReport.Pushed
			syncRan = true
		}
	}

	// Step 3: Apply dotfiles if requested. Skipped when sync already
	// applied (to avoid double-applying the same state).
	if dot.ApplyOnInit && !syncRan {
		applyDeps := ApplyDeps{
			ExecFn: deps.ExecFn,
			State:  deps.State,
			Audit:  deps.Audit,
			DryRun: false,
		}
		applyReport, err := Apply(ctx, applyDeps)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("apply: %v", err))
		} else {
			report.Applied = applyReport.Applied
		}
	}

	// Step 3: Track each managed path. Sensitive paths are SKIPPED, not failed.
	addDeps := AddDeps{
		ExecFn: deps.ExecFn,
		State:  deps.State,
		Audit:  deps.Audit,
	}
	for _, p := range dot.ManagedPaths {
		// We do a quick sensitivity pre-check here so we can collect skipped
		// paths into a distinct list. The full validateManagedPath would also
		// reject these, but with a different error.
		if isSensitivePath(p) {
			report.SkippedPaths = append(report.SkippedPaths, p)
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("skipped sensitive path %q (run 'nexus dotfiles add %s --force' to override)", p, p))
			continue
		}

		if _, err := Add(ctx, p, addDeps, false); err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("add %q: %v", p, err))
			continue
		}
		report.AddedPaths = append(report.AddedPaths, p)
	}

	return report
}
