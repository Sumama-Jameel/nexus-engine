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
	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeExec is a programmable ExecFunc for mode tests. It returns canned
// responses or errors — same pattern as pushScript in internal/dotfiles.
type fakeExec struct {
	t         *testing.T
	responses map[string]string
	errs      map[string]error
	callLog   []string
}

func (f *fakeExec) run(ctx context.Context, cmd string, args ...string) (string, error) {
	f.t.Helper()
	key := strings.Join(append([]string{cmd}, args...), " ")
	f.callLog = append(f.callLog, key)
	if err, ok := f.errs[key]; ok {
		return "", err
	}
	if resp, ok := f.responses[key]; ok {
		return resp, nil
	}
	return "", nil
}

func newFake(t *testing.T) *fakeExec {
	return &fakeExec{t: t, responses: make(map[string]string), errs: make(map[string]error)}
}

// newTracker creates a StateTracker in a temp HOME so we don't
// pollute the real ~/.nexus/state.json. Reuses the pattern from
// internal/engine/state_test.go.
func newTracker(t *testing.T) *engine.StateTracker {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	tracker, err := engine.NewStateTracker()
	if err != nil {
		t.Fatalf("NewStateTracker: %v", err)
	}
	return tracker
}

// ---------------------------------------------------------------------------
// Builtins
// ---------------------------------------------------------------------------

func TestBuiltinNames(t *testing.T) {
	names := BuiltinNames()
	want := []string{"dev", "gamer", "work"}
	if len(names) != len(want) {
		t.Fatalf("got %d names, want %d: %v", len(names), len(want), names)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, n, want[i])
		}
	}
}

func TestLoadBuiltin(t *testing.T) {
	for _, name := range BuiltinNames() {
		m, err := loadBuiltin(name)
		if err != nil {
			t.Fatalf("loadBuiltin(%q): %v", name, err)
		}
		if !m.Builtin {
			t.Errorf("%q.Builtin = false", name)
		}
		if m.Name != name {
			t.Errorf("%q.Name = %q", name, m.Name)
		}
		if m.Profile == "" {
			t.Errorf("%q has no profile", name)
		}
	}
}

func TestResolveBuiltin(t *testing.T) {
	for _, name := range BuiltinNames() {
		m, err := Resolve(name)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", name, err)
		}
		if !m.Builtin {
			t.Errorf("Resolve(%q).Builtin = false", name)
		}
	}
}

func TestResolveUnknown(t *testing.T) {
	_, err := Resolve("nonexistent")
	if err == nil {
		t.Fatal("Resolve('nonexistent') should error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say 'not found', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// User override
// ---------------------------------------------------------------------------

func TestResolveUserOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".nexus", "modes")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `name: gamer
description: Custom override
profile: custom
dotfiles_source: ""
stop_services: [docker]
start_services: []
os_tweaks:
  linux:
    cpu_governor: performance
  windows:
    power_plan: high_performance
`

	if err := os.WriteFile(filepath.Join(dir, "gamer.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := Resolve("gamer")
	if err != nil {
		t.Fatalf("Resolve('gamer') with user override: %v", err)
	}
	if m.Builtin {
		t.Error("user mode should have Builtin=false")
	}
	// Wait — loadUserMode reads the file and sets Builtin=false. But
	// loadUserMode is called by Resolve which checks user first. The
	// user mode returns with Builtin=false. Good.
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	modes, err := List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(modes) != 3 {
		t.Fatalf("expected 3 modes (no user overrides), got %d", len(modes))
	}
}

func TestListWithUserOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".nexus", "modes")
	os.MkdirAll(dir, 0755)

	// Write a user mode with a unique name
	content := `name: custom
description: User-only
profile: custom
`
	os.WriteFile(filepath.Join(dir, "custom.yaml"), []byte(content), 0644)

	modes, err := List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(modes) != 4 {
		t.Fatalf("expected 4 modes (3 builtins + 1 user), got %d", len(modes))
	}
}

// ---------------------------------------------------------------------------
// validate
// ---------------------------------------------------------------------------

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		m    Mode
		want string
	}{
		{name: "empty name", m: Mode{}, want: "name is required"},
		{name: "name with slash", m: Mode{Name: "a/b"}, want: "name"},
		{name: "no profile", m: Mode{Name: "t"}, want: "profile is required"},
		{name: "empty stop svc", m: Mode{Name: "t", Profile: "p", StopServices: []string{""}}, want: "empty entry"},
		{name: "empty start svc", m: Mode{Name: "t", Profile: "p", StartServices: []string{""}}, want: "empty entry"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(&tc.m)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestValidateOK(t *testing.T) {
	m := Mode{Name: "test", Profile: "base"}
	if err := validate(&m); err != nil {
		t.Fatal("unexpected error:", err)
	}
}

// ---------------------------------------------------------------------------
// isServiceAllowed
// ---------------------------------------------------------------------------

func TestIsServiceAllowed(t *testing.T) {
	tests := []struct {
		svc  string
		os   string
		want bool
	}{
		{"podman", "linux", true},
		{"docker", "linux", true},
		{"sshd", "linux", true},
		{"network-manager", "linux", false},
		{"network-manager", "windows", false},
		{"com.docker.service", "windows", true},
		{"unknown", "darwin", false},
	}
	for _, tc := range tests {
		t.Run(tc.svc+"_"+tc.os, func(t *testing.T) {
			got := IsServiceAllowed(tc.svc, tc.os)
			if got != tc.want {
				t.Errorf("IsServiceAllowed(%q, %q) = %v, want %v", tc.svc, tc.os, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// resolvePowerPlan
// ---------------------------------------------------------------------------

func TestResolvePowerPlan(t *testing.T) {
	tests := []struct {
		alias string
		want  string
		ok    bool
	}{
		{"balanced", "381b4222-f694-41f0-9685-ff5bb260df2e", true},
		{"BALANCED", "381b4222-f694-41f0-9685-ff5bb260df2e", true},
		{"high_performance", "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c", true},
		{"high-performance", "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c", true},
		{"performance", "8c5e7fda-e8bf-4a96-9a85-a6e23a8c635c", true},
		{"power_saver", "a1841308-3541-4fab-bc81-f71556f20b4a", true},
		{"power-saver", "a1841308-3541-4fab-bc81-f71556f20b4a", true},
		{"saver", "a1841308-3541-4fab-bc81-f71556f20b4a", true},
		{"raw", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.alias, func(t *testing.T) {
			guid, err := resolvePowerPlan(tc.alias)
			if tc.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.ok && guid != tc.want {
				t.Errorf("got %q, want %q", guid, tc.want)
			}
			if !tc.ok && err == nil {
				t.Errorf("expected error for %q", tc.alias)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Apply — dry-run
// ---------------------------------------------------------------------------

func TestApplyDryRun(t *testing.T) {
	f := newFake(t)
	deps := ApplyDeps{
		ExecFn:       f.run,
		GOOS:         "linux",
		ApplyProfile: func(_ context.Context, _ string, _ bool) error { return nil },
	}
	report, err := Apply(context.Background(), deps, "dev", ApplyOpts{DryRun: true})
	if err != nil {
		t.Fatalf("Apply dry-run: %v", err)
	}
	if !report.DryRun {
		t.Errorf("DryRun should be true")
	}
	if len(report.Plan) == 0 {
		t.Error("dry run should have a plan")
	}
	if len(f.callLog) != 0 {
		t.Errorf("dry run should not execute commands, got %d calls", len(f.callLog))
	}
}

// ---------------------------------------------------------------------------
// Apply — happy path
// ---------------------------------------------------------------------------

func TestApplyHappy(t *testing.T) {
	f := newFake(t)
	deps := ApplyDeps{
		ExecFn:       f.run,
		GOOS:         "linux",
		ApplyProfile: func(_ context.Context, _ string, _ bool) error { return nil },
	}
	report, err := Apply(context.Background(), deps, "dev", ApplyOpts{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if report.Outcome != "success" {
		t.Errorf("outcome = %q, want success", report.Outcome)
	}
	if len(report.Steps) == 0 {
		t.Error("should have at least one step")
	}
}

// ---------------------------------------------------------------------------
// Apply — unlisted service (default deny)
// ---------------------------------------------------------------------------

func TestApplyUnlistedDeny(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, ".nexus", "modes")
	os.MkdirAll(dir, 0755)
	content := `name: risky
description: Has an unlisted service
profile: base
stop_services: [network-manager]
`
	os.WriteFile(filepath.Join(dir, "risky.yaml"), []byte(content), 0644)

	deps := ApplyDeps{
		ExecFn:       func(_ context.Context, _ string, _ ...string) (string, error) { return "", nil },
		GOOS:         "linux",
		ApplyProfile: func(_ context.Context, _ string, _ bool) error { return nil },
	}
	_, err := Apply(context.Background(), deps, "risky", ApplyOpts{})
	if err == nil {
		t.Fatal("should deny unlisted service")
	}
	if !strings.Contains(err.Error(), "not in the allowlist") {
		t.Errorf("error should mention allowlist, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Apply — unlisted service allowed (Option C)
// ---------------------------------------------------------------------------

func TestApplyAllowUnlisted(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".nexus", "modes")
	os.MkdirAll(dir, 0755)
	content := `name: risky
description: Has an unlisted service
profile: base
stop_services: [network-manager]
`
	os.WriteFile(filepath.Join(dir, "risky.yaml"), []byte(content), 0644)

	deps := ApplyDeps{
		ExecFn:       func(_ context.Context, _ string, _ ...string) (string, error) { return "", nil },
		GOOS:         "linux",
		ApplyProfile: func(_ context.Context, _ string, _ bool) error { return nil },
	}
	_, err := Apply(context.Background(), deps, "risky", ApplyOpts{AllowUnlistedServices: true})
	if err != nil {
		t.Fatalf("should succeed with AllowUnlistedServices: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Rollback
// ---------------------------------------------------------------------------

func TestRollbackNoPrevious(t *testing.T) {
	tracker := newTracker(t)
	deps := ApplyDeps{
		ExecFn:       func(_ context.Context, _ string, _ ...string) (string, error) { return "", nil },
		GOOS:         "linux",
		State:        tracker,
		ApplyProfile: func(_ context.Context, _ string, _ bool) error { return nil },
	}
	_, err := Rollback(context.Background(), deps, ApplyOpts{})
	if err == nil {
		t.Fatal("rollback without previous should error")
	}
	if !strings.Contains(err.Error(), "no previous") {
		t.Errorf("should say 'no previous', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// StateTracker — RecordModeApply / GetActiveMode / GetModeState
// ---------------------------------------------------------------------------

func TestRecordModeApplyState(t *testing.T) {
	tracker := newTracker(t)

	if err := tracker.RecordModeApply("", "dev"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if a := tracker.GetActiveMode(); a != "dev" {
		t.Errorf("GetActiveMode() = %q, want 'dev'", a)
	}
	if p := tracker.GetModeState().Previous; p != "" {
		t.Errorf("Previous = %q, want empty", p)
	}

	if err := tracker.RecordModeApply("dev", "gamer"); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if a := tracker.GetActiveMode(); a != "gamer" {
		t.Errorf("GetActiveMode() = %q, want 'gamer'", a)
	}
	if p := tracker.GetModeState().Previous; p != "dev" {
		t.Errorf("Previous = %q, want 'dev'", p)
	}
	if h := tracker.GetModeState().History; len(h) != 2 {
		t.Errorf("History should have 2 entries, got %d", len(h))
	}
}
