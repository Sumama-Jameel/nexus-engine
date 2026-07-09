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
	"strings"
	"testing"
)

// ─── Apply: input validation & preconditions ──────────────────────────────

func TestApply_NilExecFn(t *testing.T) {
	_, err := Apply(context.Background(), ApplyDeps{ExecFn: nil})
	if err == nil {
		t.Fatal("expected error when ExecFn is nil")
	}
	if !strings.Contains(err.Error(), "ExecFn must not be nil") {
		t.Errorf("expected 'must not be nil' error, got: %v", err)
	}
}

func TestApply_NoSourceBound(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called when no source is bound (got %s %v)", cmd, args)
		return "", nil
	}
	tracker := withTempHome(t, "") // empty source

	_, err := Apply(context.Background(),
		ApplyDeps{ExecFn: execFn, State: tracker})
	if err == nil {
		t.Fatal("expected error when no source is bound")
	}
	if !strings.Contains(err.Error(), "no dotfile source bound") {
		t.Errorf("expected 'no dotfile source bound' error, got: %v", err)
	}
}

func TestApply_NoStateStillBlocks(t *testing.T) {
	// Without a StateTracker, source defaults to "" → blocked.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called when no source is bound (got %s %v)", cmd, args)
		return "", nil
	}

	_, err := Apply(context.Background(),
		ApplyDeps{ExecFn: execFn, State: nil})
	if err == nil {
		t.Fatal("expected error when State is nil")
	}
	if !strings.Contains(err.Error(), "no dotfile source bound") {
		t.Errorf("expected 'no dotfile source bound' error, got: %v", err)
	}
}

// ─── Apply: dry-run ────────────────────────────────────────────────────────

func TestApply_DryRunNoDifferences(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "diff" {
			return "", nil // empty diff = no differences
		}
		t.Errorf("unexpected ExecFn call: %s %v", cmd, args)
		return "", nil
	}
	tracker := withTempHome(t, "https://github.com/foo/bar")

	report, err := Apply(context.Background(),
		ApplyDeps{ExecFn: execFn, State: tracker, DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Applied {
		t.Errorf("Applied should be false in dry-run")
	}
	if report.DiffOutput != "(no differences)" {
		t.Errorf("expected DiffOutput='(no differences)', got %q", report.DiffOutput)
	}
	if report.Source != "https://github.com/foo/bar" {
		t.Errorf("expected Source to be populated, got %q", report.Source)
	}
}

func TestApply_DryRunWithDiff(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "diff" {
			return "--- a/.zshrc\n+++ b/.zshrc\n@@ -1 +1 @@\n-old\n+new\n", nil
		}
		t.Errorf("unexpected ExecFn call: %s %v", cmd, args)
		return "", nil
	}
	tracker := withTempHome(t, "https://github.com/foo/bar")

	report, err := Apply(context.Background(),
		ApplyDeps{ExecFn: execFn, State: tracker, DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Applied {
		t.Errorf("Applied should be false in dry-run")
	}
	if !strings.Contains(report.DiffOutput, "old") || !strings.Contains(report.DiffOutput, "new") {
		t.Errorf("expected diff content in DiffOutput, got %q", report.DiffOutput)
	}
}

func TestApply_DryRunDiffExit1(t *testing.T) {
	// Per the doc: chezmoi diff exits 1 when differences exist. We map that
	// to a placeholder message — the diff content itself is lost.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("chezmoi: exit status 1")
	}
	tracker := withTempHome(t, "https://github.com/foo/bar")

	report, err := Apply(context.Background(),
		ApplyDeps{ExecFn: execFn, State: tracker, DryRun: true})
	if err != nil {
		t.Fatalf("expected no error for diff exit 1, got: %v", err)
	}
	if !strings.Contains(report.DiffOutput, "differences detected") {
		t.Errorf("expected 'differences detected' placeholder, got %q", report.DiffOutput)
	}
}

// ─── Apply: actual apply ───────────────────────────────────────────────────

func TestApply_Success(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "apply" {
			return "applied 3 files", nil
		}
		t.Errorf("unexpected ExecFn call: %s %v", cmd, args)
		return "", nil
	}
	tracker := withTempHome(t, "https://github.com/foo/bar")

	report, err := Apply(context.Background(),
		ApplyDeps{ExecFn: execFn, State: tracker})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Applied {
		t.Errorf("expected Applied=true")
	}
	if report.RawOutput != "applied 3 files" {
		t.Errorf("expected RawOutput to be captured, got %q", report.RawOutput)
	}
	if report.AppliedAt == "" {
		t.Errorf("expected AppliedAt to be populated from state")
	}

	// State should record LastAppliedAt.
	st := tracker.GetDotfilesState()
	if st.LastAppliedAt.IsZero() {
		t.Errorf("expected LastAppliedAt to be set in state")
	}
	if st.Source != "https://github.com/foo/bar" {
		t.Errorf("expected Source to be preserved, got %q", st.Source)
	}
}

func TestApply_ExecFailure(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("chezmoi: apply failed: permission denied")
	}
	tracker := withTempHome(t, "https://github.com/foo/bar")

	_, err := Apply(context.Background(),
		ApplyDeps{ExecFn: execFn, State: tracker})
	if err == nil {
		t.Fatal("expected error when chezmoi apply fails")
	}
	if !strings.Contains(err.Error(), "chezmoi apply failed") {
		t.Errorf("expected 'chezmoi apply failed' error, got: %v", err)
	}
}

// ─── Status ────────────────────────────────────────────────────────────────

func TestStatus_NilExecFn(t *testing.T) {
	_, err := Status(context.Background(), ApplyDeps{ExecFn: nil})
	if err == nil {
		t.Fatal("expected error when ExecFn is nil")
	}
	if !strings.Contains(err.Error(), "ExecFn must not be nil") {
		t.Errorf("expected 'must not be nil' error, got: %v", err)
	}
}

func TestStatus_Success(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "status" {
			return "M .zshrc\n", nil
		}
		t.Errorf("unexpected ExecFn call: %s %v", cmd, args)
		return "", nil
	}

	out, err := Status(context.Background(), ApplyDeps{ExecFn: execFn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "M .zshrc\n" {
		t.Errorf("expected output to be returned verbatim, got %q", out)
	}
}

func TestStatus_Exit1MeansUnapplied(t *testing.T) {
	// Per the doc: chezmoi status exits 1 when there are unapplied changes.
	// We surface a placeholder rather than treating it as a failure.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("chezmoi: exit status 1")
	}

	out, err := Status(context.Background(), ApplyDeps{ExecFn: execFn})
	if err != nil {
		t.Fatalf("expected no error for exit status 1, got: %v", err)
	}
	if !strings.Contains(out, "unapplied changes detected") {
		t.Errorf("expected 'unapplied changes detected' placeholder, got %q", out)
	}
}

func TestStatus_OtherError(t *testing.T) {
	// Errors other than "exit status 1" are real failures.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("TIMEOUT: chezmoi exceeded 60s")
	}

	_, err := Status(context.Background(), ApplyDeps{ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error for non-exit-1 failure")
	}
	if !strings.Contains(err.Error(), "chezmoi status failed") {
		t.Errorf("expected 'chezmoi status failed' error, got: %v", err)
	}
}

// ─── Diff ──────────────────────────────────────────────────────────────────

func TestDiff_NilExecFn(t *testing.T) {
	_, err := Diff(context.Background(), ApplyDeps{ExecFn: nil})
	if err == nil {
		t.Fatal("expected error when ExecFn is nil")
	}
	if !strings.Contains(err.Error(), "ExecFn must not be nil") {
		t.Errorf("expected 'must not be nil' error, got: %v", err)
	}
}

func TestDiff_NoDifferences(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", nil
	}

	out, err := Diff(context.Background(), ApplyDeps{ExecFn: execFn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "(no differences)" {
		t.Errorf("expected '(no differences)', got %q", out)
	}
}

func TestDiff_WithDifferences(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "--- a/.zshrc\n+++ b/.zshrc\n", nil
	}

	out, err := Diff(context.Background(), ApplyDeps{ExecFn: execFn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "--- a/.zshrc\n+++ b/.zshrc\n" {
		t.Errorf("expected diff to be returned verbatim, got %q", out)
	}
}

func TestDiff_Exit1MeansDifferencesDetected(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("chezmoi: exit status 1")
	}

	out, err := Diff(context.Background(), ApplyDeps{ExecFn: execFn})
	if err != nil {
		t.Fatalf("expected no error for exit status 1, got: %v", err)
	}
	if !strings.Contains(out, "differences detected") {
		t.Errorf("expected 'differences detected' placeholder, got %q", out)
	}
}

func TestDiff_OtherError(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("chezmoi: repository not found")
	}

	_, err := Diff(context.Background(), ApplyDeps{ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error for non-exit-1 failure")
	}
	if !strings.Contains(err.Error(), "chezmoi diff failed") {
		t.Errorf("expected 'chezmoi diff failed' error, got: %v", err)
	}
}
