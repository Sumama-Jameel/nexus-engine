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

	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
)

// fakePackageManager is a minimal PackageManager implementation for tests.
//
// Behavior is controlled by the exported fields:
//   - installErr: returned by Install (nil = success)
//   - refreshErr: returned by RefreshIndex (nil = success)
//   - isInstalled: returned by IsInstalled (lookup table)
//
// The Install implementation returns a PackageResult with Success=true when
// installErr is nil, matching what the real orchestrator would receive from
// a healthy package manager.
type fakePackageManager struct {
	name        string
	installErr  error
	refreshErr  error
	isInstalled map[string]bool
}

func (f *fakePackageManager) RefreshIndex(ctx context.Context) error {
	return f.refreshErr
}

func (f *fakePackageManager) Install(ctx context.Context, pkgs []string) ([]installer.PackageResult, error) {
	if f.installErr != nil {
		return nil, f.installErr
	}
	results := make([]installer.PackageResult, 0, len(pkgs))
	for _, p := range pkgs {
		results = append(results, installer.PackageResult{
			Package: p,
			Action:  "install",
			Success: true,
		})
	}
	return results, nil
}

func (f *fakePackageManager) Remove(ctx context.Context, pkgs []string) ([]installer.PackageResult, error) {
	return nil, nil
}

func (f *fakePackageManager) Update(ctx context.Context, pkgs []string) ([]installer.PackageResult, error) {
	return nil, nil
}

func (f *fakePackageManager) IsInstalled(ctx context.Context, pkg string) bool {
	return f.isInstalled[pkg]
}

func (f *fakePackageManager) ListInstalled(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (f *fakePackageManager) Search(ctx context.Context, query string) ([]string, error) {
	return nil, nil
}

func (f *fakePackageManager) Name() string {
	return f.name
}

// ─── InstallChezmoi: input validation ─────────────────────────────────────

func TestInstallChezmoi_NilPM(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", nil
	}

	_, err := InstallChezmoi(context.Background(),
		InstallDeps{PM: nil, ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error when PM is nil")
	}
	if !strings.Contains(err.Error(), "PM must not be nil") {
		t.Errorf("expected 'PM must not be nil' error, got: %v", err)
	}
}

func TestInstallChezmoi_NilExecFn(t *testing.T) {
	pm := &fakePackageManager{name: "apt"}

	_, err := InstallChezmoi(context.Background(),
		InstallDeps{PM: pm, ExecFn: nil})
	if err == nil {
		t.Fatal("expected error when ExecFn is nil")
	}
	if !strings.Contains(err.Error(), "ExecFn must not be nil") {
		t.Errorf("expected 'ExecFn must not be nil' error, got: %v", err)
	}
}

// ─── InstallChezmoi: install step ──────────────────────────────────────────

func TestInstallChezmoi_PMInstallFails(t *testing.T) {
	// The Orchestrator swallows non-foundation package failures (chezmoi is
	// PriorityTool, not PriorityFoundation). It records Success=false in
	// result.Packages and returns nil error. The post-install Detect probe
	// is what actually surfaces the failure: chezmoi is not on PATH because
	// the install failed.
	pm := &fakePackageManager{
		name:       "apt",
		installErr: errors.New("apt-get: unable to locate package chezmoi"),
	}
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		// Detect probe finds no chezmoi.
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "--version" {
			return "", errors.New(`exec: "chezmoi": executable file not found in $PATH`)
		}
		return "", nil
	}

	res, err := InstallChezmoi(context.Background(),
		InstallDeps{PM: pm, ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error when PM.Install fails (caught by Detect)")
	}
	if res == nil {
		t.Fatal("expected non-nil InstallResult")
	}
	if res.Installed {
		t.Errorf("expected Installed=false when Detect fails, got true")
	}
	if !strings.Contains(err.Error(), "binary is not on PATH") {
		t.Errorf("expected 'binary is not on PATH' error, got: %v", err)
	}
}

// ─── InstallChezmoi: post-install verification ────────────────────────────

func TestInstallChezmoi_PostInstallVerifyFails(t *testing.T) {
	pm := &fakePackageManager{name: "apt"}
	// ExecFn fails on the verification probe (chezmoi --version).
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "--version" {
			return "", errors.New("chezmoi: command not found after install")
		}
		return "", nil
	}

	res, err := InstallChezmoi(context.Background(),
		InstallDeps{PM: pm, ExecFn: execFn})

	// Install succeeded but verification failed — we expect BOTH a non-nil
	// error AND a partial InstallResult reporting Installed=true.
	if err == nil {
		t.Fatal("expected error when post-install verification fails")
	}
	if res == nil {
		t.Fatal("expected non-nil InstallResult even when verification fails")
	}
	if !res.Installed {
		t.Errorf("expected Installed=true (the install itself succeeded), got false")
	}
	if !strings.Contains(err.Error(), "post-install verification failed") {
		t.Errorf("expected 'post-install verification failed' error, got: %v", err)
	}
}

func TestInstallChezmoi_VerifyReportsNotInstalled(t *testing.T) {
	pm := &fakePackageManager{name: "apt"}
	// PM reports install success, but the post-install chezmoi --version
	// returns the "not installed" marker — typically a broken PATH.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("exec: \"chezmoi\": executable file not found in $PATH")
	}

	res, err := InstallChezmoi(context.Background(),
		InstallDeps{PM: pm, ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error when verification reports not installed")
	}
	if res == nil {
		t.Fatal("expected non-nil InstallResult")
	}
	if res.Installed {
		t.Errorf("expected Installed=false (binary not on PATH), got true")
	}
	if !strings.Contains(err.Error(), "binary is not on PATH") {
		t.Errorf("expected 'binary is not on PATH' error, got: %v", err)
	}
}

func TestInstallChezmoi_Success(t *testing.T) {
	pm := &fakePackageManager{name: "apt"}
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "--version" {
			return "chezmoi version 2.50.0\n", nil
		}
		return "", nil
	}
	tracker := withTempHome(t, "")

	res, err := InstallChezmoi(context.Background(),
		InstallDeps{PM: pm, ExecFn: execFn, State: tracker})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil InstallResult on success")
	}
	if !res.Installed {
		t.Errorf("expected Installed=true, got false")
	}
	if res.Version != "2.50.0" {
		t.Errorf("expected Version='2.50.0', got %q", res.Version)
	}
	if res.PackageManager != "apt" {
		t.Errorf("expected PackageManager='apt', got %q", res.PackageManager)
	}

	// State should record the install.
	st := tracker.GetDotfilesState()
	if !st.Installed {
		t.Errorf("expected state.Dotfiles.Installed=true after success")
	}
	if st.Version != "2.50.0" {
		t.Errorf("expected state.Dotfiles.Version='2.50.0', got %q", st.Version)
	}
}

func TestInstallChezmoi_Success_DryRun(t *testing.T) {
	// DryRun=true means we don't actually exec anything but still verify.
	pm := &fakePackageManager{name: "apt"}
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "--version" {
			return "chezmoi version 2.50.0\n", nil
		}
		// Anything else should not be called in dry-run.
		t.Errorf("unexpected ExecFn call in dry-run: %s %v", cmd, args)
		return "", nil
	}

	res, err := InstallChezmoi(context.Background(),
		InstallDeps{PM: pm, ExecFn: execFn, DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error in dry-run: %v", err)
	}
	if !res.Installed {
		t.Errorf("expected Installed=true in dry-run")
	}
}
