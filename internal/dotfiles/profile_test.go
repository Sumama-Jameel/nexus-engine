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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
)

// ─── ApplyFromProfile: no-op cases ─────────────────────────────────────────

func TestApplyFromProfile_NilProfile(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called for nil profile")
		return "", nil
	}

	report := ApplyFromProfile(context.Background(), nil,
		ProfileDeps{ExecFn: execFn})
	if report == nil {
		t.Fatal("expected non-nil report even for nil profile")
	}
	if report.SourceBound || report.Applied || len(report.AddedPaths) > 0 {
		t.Errorf("expected all flags false/empty for nil profile, got %+v", report)
	}
}

func TestApplyFromProfile_NoDotfilesSection(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called when Dotfiles is nil")
		return "", nil
	}

	profile := &manifest.NexusProfile{
		Name: "base-dev",
		// Dotfiles deliberately nil
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn})
	if report.SourceBound || report.Applied || len(report.AddedPaths) > 0 {
		t.Errorf("expected all flags false/empty when Dotfiles is nil, got %+v", report)
	}
}

func TestApplyFromProfile_NilExecFn(t *testing.T) {
	profile := &manifest.NexusProfile{
		Name:     "base-dev",
		Dotfiles: &manifest.DotfilesSpec{Source: "https://github.com/foo/bar"},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: nil})
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Warnings) == 0 {
		t.Error("expected a warning when ExecFn is nil")
	}
	if !strings.Contains(report.Warnings[0], "ExecFunc is nil") {
		t.Errorf("expected 'ExecFunc is nil' warning, got %q", report.Warnings[0])
	}
}

// ─── ApplyFromProfile: source binding ──────────────────────────────────────

func TestApplyFromProfile_BindSourceSuccess(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) >= 2 && args[0] == "init" {
			return "Initialized", nil
		}
		return "", nil
	}
	tracker := withTempHome(t, "")

	profile := &manifest.NexusProfile{
		Name:     "base-dev",
		Dotfiles: &manifest.DotfilesSpec{Source: "https://github.com/foo/bar"},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if err := reportErr(report); err != nil {
		t.Skipf("network may be unavailable: %v", err)
	}
	if !report.SourceBound {
		t.Errorf("expected SourceBound=true, got %+v", report)
	}
	if st := tracker.GetDotfilesState(); st.Source != "https://github.com/foo/bar" {
		t.Errorf("expected state Source to be set, got %q", st.Source)
	}
}

func TestApplyFromProfile_BindSourceFailsAborts(t *testing.T) {
	// Source that fails URL validation (http:// scheme) — should fail fast.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called when URL validation fails")
		return "", nil
	}

	profile := &manifest.NexusProfile{
		Name:     "base-dev",
		Dotfiles: &manifest.DotfilesSpec{Source: "http://github.com/foo/bar"},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn})
	if report.SourceBound {
		t.Errorf("expected SourceBound=false when bind fails")
	}
	if len(report.Warnings) == 0 {
		t.Fatal("expected a warning when bind fails")
	}
	if !strings.Contains(report.Warnings[0], "bind source") {
		t.Errorf("expected 'bind source' in warning, got %q", report.Warnings[0])
	}
}

// ─── ApplyFromProfile: apply on init ───────────────────────────────────────

func TestApplyFromProfile_ApplyOnInitAfterBind(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		switch {
		case cmd == "chezmoi" && len(args) == 2 && args[0] == "init":
			return "Initialized", nil
		case cmd == "chezmoi" && len(args) == 1 && args[0] == "apply":
			return "applied", nil
		default:
			return "", nil
		}
	}
	tracker := withTempHome(t, "")

	profile := &manifest.NexusProfile{
		Name: "base-dev",
		Dotfiles: &manifest.DotfilesSpec{
			Source:      "https://github.com/foo/bar",
			ApplyOnInit: true,
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if err := reportErr(report); err != nil {
		t.Skipf("network may be unavailable: %v", err)
	}
	if !report.SourceBound {
		t.Errorf("expected SourceBound=true")
	}
	if !report.Applied {
		t.Errorf("expected Applied=true when ApplyOnInit=true")
	}
}

func TestApplyFromProfile_ApplyOnInitWithoutSource(t *testing.T) {
	// ApplyOnInit=true with no Source set: Apply runs using the source
	// already bound in state.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "apply" {
			return "applied", nil
		}
		return "", nil
	}
	tracker := withTempHome(t, "https://github.com/foo/bar") // pre-bound

	profile := &manifest.NexusProfile{
		Name: "base-dev",
		Dotfiles: &manifest.DotfilesSpec{
			Source:      "", // no source in profile
			ApplyOnInit: true,
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if !report.Applied {
		t.Errorf("expected Applied=true, warnings=%v", report.Warnings)
	}
	if report.SourceBound {
		t.Errorf("expected SourceBound=false (no source in profile)")
	}
}

func TestApplyFromProfile_ApplyFailsIsWarning(t *testing.T) {
	// Apply errors are accumulated as warnings, not fatal — the rest of the
	// pipeline (Add) must still run.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		switch {
		case cmd == "chezmoi" && len(args) == 2 && args[0] == "init":
			return "Initialized", nil
		case cmd == "chezmoi" && len(args) == 1 && args[0] == "apply":
			return "", errors.New("chezmoi apply failed")
		case cmd == "chezmoi" && len(args) == 3 && args[0] == "add":
			return "", nil
		default:
			return "", nil
		}
	}
	tracker := withTempHome(t, "")

	home, err := userHomeDirOrSkip(t)
	if err != nil {
		t.Skip(err)
	}

	profile := &manifest.NexusProfile{
		Name: "base-dev",
		Dotfiles: &manifest.DotfilesSpec{
			Source:       "https://github.com/foo/bar",
			ApplyOnInit:  true,
			ManagedPaths: []string{filepath.Join(home, ".zshrc")},
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if err := reportErr(report); err != nil {
		t.Skipf("network may be unavailable: %v", err)
	}

	if report.Applied {
		t.Errorf("expected Applied=false when apply fails")
	}
	// The Add step should still have run despite the apply failure.
	if len(report.AddedPaths) != 1 {
		t.Errorf("expected 1 added path (apply failure should not block), got %d: %+v",
			len(report.AddedPaths), report)
	}
}

// ─── ApplyFromProfile: managed paths ───────────────────────────────────────

func TestApplyFromProfile_ManagedPathsAdd(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) >= 2 && args[0] == "add" {
			return "", nil
		}
		return "", nil
	}
	tracker := withTempHome(t, "")

	home, err := userHomeDirOrSkip(t)
	if err != nil {
		t.Skip(err)
	}

	profile := &manifest.NexusProfile{
		Name: "base-dev",
		Dotfiles: &manifest.DotfilesSpec{
			ManagedPaths: []string{
				filepath.Join(home, ".zshrc"),
				filepath.Join(home, ".gitconfig"),
			},
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if len(report.AddedPaths) != 2 {
		t.Errorf("expected 2 added paths, got %d: %+v", len(report.AddedPaths), report)
	}
	if len(report.SkippedPaths) != 0 {
		t.Errorf("expected no skipped paths, got %v", report.SkippedPaths)
	}
}

func TestApplyFromProfile_SensitivePathsSkipped(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) >= 2 && args[0] == "add" {
			// Should not be called for sensitive paths.
			t.Errorf("ExecFn should not be called for sensitive paths (got add %v)", args)
			return "", nil
		}
		return "", nil
	}
	tracker := withTempHome(t, "")

	home, err := userHomeDirOrSkip(t)
	if err != nil {
		t.Skip(err)
	}

	profile := &manifest.NexusProfile{
		Name: "base-dev",
		Dotfiles: &manifest.DotfilesSpec{
			ManagedPaths: []string{
				filepath.Join(home, ".ssh", "id_rsa"),
				filepath.Join(home, ".aws", "credentials"),
			},
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if len(report.SkippedPaths) != 2 {
		t.Errorf("expected 2 skipped paths, got %d: %+v", len(report.SkippedPaths), report)
	}
	if len(report.AddedPaths) != 0 {
		t.Errorf("expected 0 added paths, got %d", len(report.AddedPaths))
	}
	if len(report.Warnings) != 2 {
		t.Errorf("expected 2 warnings (one per skipped path), got %d", len(report.Warnings))
	}
	for _, w := range report.Warnings {
		if !strings.Contains(w, "sensitive") {
			t.Errorf("expected warning to mention 'sensitive', got %q", w)
		}
		if !strings.Contains(w, "--force") {
			t.Errorf("expected warning to mention --force, got %q", w)
		}
	}
}

func TestApplyFromProfile_MixedPathsAddedAndSkipped(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) >= 2 && args[0] == "add" {
			return "", nil
		}
		return "", nil
	}
	tracker := withTempHome(t, "")

	home, err := userHomeDirOrSkip(t)
	if err != nil {
		t.Skip(err)
	}

	profile := &manifest.NexusProfile{
		Name: "base-dev",
		Dotfiles: &manifest.DotfilesSpec{
			ManagedPaths: []string{
				filepath.Join(home, ".zshrc"),                  // safe → added
				filepath.Join(home, ".ssh", "id_ed25519"),      // sensitive → skipped
				filepath.Join(home, ".gitconfig"),              // safe → added
				filepath.Join(home, ".netrc"),                  // sensitive → skipped
			},
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if len(report.AddedPaths) != 2 {
		t.Errorf("expected 2 added paths, got %d: %v", len(report.AddedPaths), report.AddedPaths)
	}
	if len(report.SkippedPaths) != 2 {
		t.Errorf("expected 2 skipped paths, got %d: %v", len(report.SkippedPaths), report.SkippedPaths)
	}
}

func TestApplyFromProfile_AddFailureIsWarning(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) >= 2 && args[0] == "add" {
			return "", errors.New("chezmoi: file not found")
		}
		return "", nil
	}
	tracker := withTempHome(t, "")

	home, err := userHomeDirOrSkip(t)
	if err != nil {
		t.Skip(err)
	}

	profile := &manifest.NexusProfile{
		Name: "base-dev",
		Dotfiles: &manifest.DotfilesSpec{
			ManagedPaths: []string{filepath.Join(home, ".zshrc")},
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if len(report.AddedPaths) != 0 {
		t.Errorf("expected 0 added paths when chezmoi add fails, got %d", len(report.AddedPaths))
	}
	if len(report.Warnings) == 0 {
		t.Fatal("expected at least one warning when add fails")
	}
	if !strings.Contains(report.Warnings[0], "chezmoi: file not found") {
		t.Errorf("expected underlying error in warning, got %q", report.Warnings[0])
	}
}

// ─── ApplyFromProfile: full integration ────────────────────────────────────

func TestApplyFromProfile_FullPipeline(t *testing.T) {
	// All three steps active: bind source, apply on init, add managed paths.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		switch {
		case cmd == "chezmoi" && len(args) == 2 && args[0] == "init":
			return "Initialized", nil
		case cmd == "chezmoi" && len(args) == 1 && args[0] == "apply":
			return "applied 5 files", nil
		case cmd == "chezmoi" && len(args) >= 2 && args[0] == "add":
			return "", nil
		default:
			return "", nil
		}
	}
	tracker := withTempHome(t, "")

	home, err := userHomeDirOrSkip(t)
	if err != nil {
		t.Skip(err)
	}

	profile := &manifest.NexusProfile{
		Name: "full",
		Dotfiles: &manifest.DotfilesSpec{
			Source:      "https://github.com/foo/bar",
			ApplyOnInit: true,
			ManagedPaths: []string{
				filepath.Join(home, ".zshrc"),
				filepath.Join(home, ".gitconfig"),
			},
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if err := reportErr(report); err != nil {
		t.Skipf("network may be unavailable: %v", err)
	}
	if !report.SourceBound {
		t.Errorf("expected SourceBound=true")
	}
	if !report.Applied {
		t.Errorf("expected Applied=true")
	}
	if len(report.AddedPaths) != 2 {
		t.Errorf("expected 2 added paths, got %d", len(report.AddedPaths))
	}
	if len(report.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", report.Warnings)
	}
}

// ─── V8: sync_on_apply ─────────────────────────────────────────────────────

func TestApplyFromProfile_SyncOnApplyTriggersFullSync(t *testing.T) {
	// sync_on_apply=true with a bound source should trigger the full
	// pull → apply → push cycle. Verify by checking that all expected
	// chezmoi git commands were called in the right order.
	var callOrder []string
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		full := append([]string{cmd}, args...)
		callOrder = append(callOrder, strings.Join(full, " "))
		switch {
		case cmd == "chezmoi" && len(args) == 3 && args[1] == "init":
			return "Initialized", nil
		case cmd == "chezmoi" && len(args) >= 2 && args[1] == "pull":
			return "", nil
		case cmd == "chezmoi" && len(args) == 1 && args[0] == "apply":
			return "applied", nil
		case cmd == "chezmoi" && len(args) >= 2 && args[1] == "rev-parse":
			return "abc123def456", nil
		case cmd == "chezmoi" && len(args) >= 2 && args[1] == "add":
			return "", nil
		case cmd == "chezmoi" && len(args) >= 3 && args[1] == "diff":
			return "", nil
		case cmd == "chezmoi" && len(args) >= 2 && args[1] == "commit":
			return "", nil
		case cmd == "chezmoi" && len(args) >= 2 && args[1] == "push":
			return "", nil
		}
		return "", nil
	}
	tracker := withTempHome(t, "")

	profile := &manifest.NexusProfile{
		Name: "sync-on-apply",
		Dotfiles: &manifest.DotfilesSpec{
			Source:      "https://github.com/foo/bar",
			SyncOnApply: true,
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if err := reportErr(report); err != nil {
		t.Skipf("network may be unavailable: %v", err)
	}
	if !report.SourceBound {
		t.Error("expected SourceBound=true")
	}
	if !report.Applied {
		t.Error("expected Applied=true (sync applies during pull)")
	}
	if !report.Pushed {
		t.Error("expected Pushed=true (sync_on_apply triggers a push)")
	}
	if len(report.Warnings) != 0 {
		t.Errorf("expected no warnings, got %v", report.Warnings)
	}

	// Verify the sync sequence: pull → apply → push.
	mustContain := []string{
		"chezmoi git pull -- --ff-only",
		"chezmoi apply",
		"chezmoi git push",
	}
	for _, expected := range mustContain {
		found := false
		for _, call := range callOrder {
			if strings.Contains(call, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected call %q in call order:\n%s", expected,
				strings.Join(callOrder, "\n"))
		}
	}
}

func TestApplyFromProfile_SyncOnApplyWithoutSourceSkipsSync(t *testing.T) {
	// sync_on_apply=true but no source bound → sync step is skipped
	// (nothing to sync). No push should happen.
	calls := []string{}
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		full := append([]string{cmd}, args...)
		calls = append(calls, strings.Join(full, " "))
		return "", nil
	}
	tracker := withTempHome(t, "")

	profile := &manifest.NexusProfile{
		Name: "no-source",
		Dotfiles: &manifest.DotfilesSpec{
			Source:      "", // no source
			SyncOnApply: true,
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if report.Pushed {
		t.Error("expected Pushed=false when no source bound")
	}
	if report.Applied {
		t.Error("expected Applied=false when no source bound")
	}
	for _, call := range calls {
		if strings.Contains(call, "chezmoi git pull") || strings.Contains(call, "chezmoi git push") {
			t.Errorf("sync should not have run, but found call: %q", call)
		}
	}
}

func TestApplyFromProfile_SyncOnApplySkipsApplyOnInit(t *testing.T) {
	// When sync_on_apply=true AND apply_on_init=true, sync runs and
	// includes its own apply. The separate apply_on_init step must be
	// skipped to avoid double-applying the same state.
	applyCallCount := 0
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "apply" {
			applyCallCount++
		}
		return "", nil
	}
	tracker := withTempHome(t, "")

	profile := &manifest.NexusProfile{
		Name: "both-flags",
		Dotfiles: &manifest.DotfilesSpec{
			Source:      "https://github.com/foo/bar",
			SyncOnApply: true,
			ApplyOnInit: true, // both set
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if err := reportErr(report); err != nil {
		t.Skipf("network may be unavailable: %v", err)
	}
	// Apply should be called exactly once (inside sync), not twice.
	if applyCallCount != 1 {
		t.Errorf("expected exactly 1 apply call (sync includes apply), got %d", applyCallCount)
	}
	if !report.Applied {
		t.Error("expected Applied=true")
	}
	if !report.Pushed {
		t.Error("expected Pushed=true")
	}
}

func TestApplyFromProfile_SyncOnApplyFalseBackwardsCompatible(t *testing.T) {
	// sync_on_apply=false (default): no sync, only bind + apply + add.
	// This is the V7 behavior — must remain unchanged.
	var sawPull, sawPush bool
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		full := append([]string{cmd}, args...)
		s := strings.Join(full, " ")
		if strings.Contains(s, "chezmoi git pull") {
			sawPull = true
		}
		if strings.Contains(s, "chezmoi git push") {
			sawPush = true
		}
		if cmd == "chezmoi" && len(args) == 3 && args[1] == "init" {
			return "Initialized", nil
		}
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "apply" {
			return "applied", nil
		}
		return "", nil
	}
	tracker := withTempHome(t, "")

	profile := &manifest.NexusProfile{
		Name: "v7-compat",
		Dotfiles: &manifest.DotfilesSpec{
			Source:      "https://github.com/foo/bar",
			ApplyOnInit: true,
			// SyncOnApply omitted (defaults to false)
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if err := reportErr(report); err != nil {
		t.Skipf("network may be unavailable: %v", err)
	}
	if sawPull {
		t.Error("pull should NOT have been called (sync_on_apply=false)")
	}
	if sawPush {
		t.Error("push should NOT have been called (sync_on_apply=false)")
	}
	if !report.Applied {
		t.Error("expected Applied=true (apply_on_init still works)")
	}
	if report.Pushed {
		t.Error("expected Pushed=false (no sync ran)")
	}
}

func TestApplyFromProfile_SyncOnApplyFailureIsWarning(t *testing.T) {
	// If the sync step fails (e.g., network error, non-fast-forward),
	// the error must be surfaced as a warning — the remaining steps
	// (add) must still run.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		switch {
		case cmd == "chezmoi" && len(args) == 3 && args[1] == "init":
			return "Initialized", nil
		case cmd == "chezmoi" && len(args) >= 2 && args[1] == "pull":
			return "", errors.New("non-fast-forward")
		}
		return "", nil
	}
	tracker := withTempHome(t, "")

	home, err := userHomeDirOrSkip(t)
	if err != nil {
		t.Skip(err)
	}

	profile := &manifest.NexusProfile{
		Name: "sync-fails",
		Dotfiles: &manifest.DotfilesSpec{
			Source:      "https://github.com/foo/bar",
			SyncOnApply: true,
			ManagedPaths: []string{
				filepath.Join(home, ".zshrc"), // safe path
			},
		},
	}

	report := ApplyFromProfile(context.Background(), profile,
		ProfileDeps{ExecFn: execFn, State: tracker})
	if err := reportErr(report); err != nil {
		t.Skipf("network may be unavailable: %v", err)
	}
	// Source bound (step 1 succeeded)
	if !report.SourceBound {
		t.Error("expected SourceBound=true")
	}
	// Sync failed (step 2)
	if report.Pushed {
		t.Error("expected Pushed=false when sync fails")
	}
	// Warning recorded
	if len(report.Warnings) == 0 {
		t.Fatal("expected at least one warning when sync fails")
	}
	if !strings.Contains(report.Warnings[0], "sync:") {
		t.Errorf("expected 'sync:' prefix in warning, got %q", report.Warnings[0])
	}
	// Add step must still run despite sync failure
	if len(report.AddedPaths) != 1 {
		t.Errorf("expected 1 added path (add step should run even after sync failure), got %d",
			len(report.AddedPaths))
	}
}

// ─── helpers ────────────────────────────────────────────────────────────────────────

// reportErr returns a representative error if the report contains warnings
// that indicate network/DNS failure (used to skip tests gracefully when
// running without internet).
func reportErr(r *ProfileApplyReport) error {
	if r == nil {
		return nil
	}
	for _, w := range r.Warnings {
		if strings.Contains(w, "DNS lookup failed") {
			return errors.New(w)
		}
	}
	return nil
}

func userHomeDirOrSkip(t *testing.T) (string, error) {
	t.Helper()
	// Best-effort read of HOME so we can build absolute paths for the
	// managed-paths test cases. The withTempHome helper has already
	// redirected HOME; we read it back to construct paths.
	home := os.Getenv("HOME")
	if home == "" {
		// Windows fallback for cross-platform tests.
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		return "", errors.New("cannot determine home directory")
	}
	return home, nil
}
