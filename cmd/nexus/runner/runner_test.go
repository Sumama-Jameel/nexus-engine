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

package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Sumama-Jameel/nexus-engine/internal/bridge"
	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
)

// mockPackageManager implements installer.PackageManager for testing.
type mockPackageManager struct {
	name            string
	installFn       func(ctx context.Context, packages []string) ([]installer.PackageResult, error)
	removeFn        func(ctx context.Context, packages []string) ([]installer.PackageResult, error)
	updateFn        func(ctx context.Context, packages []string) ([]installer.PackageResult, error)
	searchFn        func(ctx context.Context, query string) ([]string, error)
	refreshIndexFn  func(ctx context.Context) error
	isInstalledFn   func(ctx context.Context, pkg string) bool
	listInstalledFn func(ctx context.Context) ([]string, error)
}

func (m *mockPackageManager) RefreshIndex(ctx context.Context) error {
	if m.refreshIndexFn != nil {
		return m.refreshIndexFn(ctx)
	}
	return nil
}

func (m *mockPackageManager) Install(ctx context.Context, packages []string) ([]installer.PackageResult, error) {
	if m.installFn != nil {
		return m.installFn(ctx, packages)
	}
	var results []installer.PackageResult
	for _, pkg := range packages {
		results = append(results, installer.PackageResult{
			Package:  pkg,
			Action:   "install",
			Success:  true,
			Verified: true,
			Duration: 100 * time.Millisecond,
		})
	}
	return results, nil
}

func (m *mockPackageManager) Remove(ctx context.Context, packages []string) ([]installer.PackageResult, error) {
	if m.removeFn != nil {
		return m.removeFn(ctx, packages)
	}
	var results []installer.PackageResult
	for _, pkg := range packages {
		results = append(results, installer.PackageResult{
			Package:  pkg,
			Action:   "remove",
			Success:  true,
			Duration: 50 * time.Millisecond,
		})
	}
	return results, nil
}

func (m *mockPackageManager) Update(ctx context.Context, packages []string) ([]installer.PackageResult, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, packages)
	}
	var results []installer.PackageResult
	for _, pkg := range packages {
		results = append(results, installer.PackageResult{
			Package:  pkg,
			Action:   "update",
			Success:  true,
			Duration: 75 * time.Millisecond,
		})
	}
	return results, nil
}

func (m *mockPackageManager) IsInstalled(ctx context.Context, pkg string) bool {
	if m.isInstalledFn != nil {
		return m.isInstalledFn(ctx, pkg)
	}
	return false
}

func (m *mockPackageManager) ListInstalled(ctx context.Context) ([]string, error) {
	if m.listInstalledFn != nil {
		return m.listInstalledFn(ctx)
	}
	return nil, nil
}

func (m *mockPackageManager) Search(ctx context.Context, query string) ([]string, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, query)
	}
	return []string{"git", "curl", "vim"}, nil
}

func (m *mockPackageManager) Name() string {
	return m.name
}

// createTestState creates a StateTracker using a temp directory.
func createTestState(t *testing.T) *engine.StateTracker {
	t.Helper()
	homeDir := t.TempDir()
	statePath := filepath.Join(homeDir, ".nexus", "state.json")
	os.MkdirAll(filepath.Dir(statePath), 0755)
	os.WriteFile(statePath, []byte(`{"version":1,"packages":{},"profiles_applied":[]}`), 0644)

	// Use the real StateTracker but we need to point it to our temp dir
	// Since NewStateTracker uses os.UserHomeDir, we override HOME
	t.Setenv("HOME", homeDir)
	tracker, err := engine.NewStateTracker()
	if err != nil {
		t.Fatalf("failed to create test state tracker: %v", err)
	}
	return tracker
}

// createTestDependencies creates a Dependencies instance with test mocks.
func createTestDependencies(t *testing.T) *Dependencies {
	t.Helper()
	pm := &mockPackageManager{name: "apt"}
	state := createTestState(t)
	env := &bridge.EnvironmentInfo{
		IsNativeLinux:  true,
		PackageManager: "apt",
		Distro:         "ubuntu",
	}

	return &Dependencies{
		PM:     pm,
		State:  state,
		Audit:  nil, // Audit is optional for tests
		Env:    env,
		Family: "debian",
		ExecFn: func(ctx context.Context, cmd string, args ...string) (string, error) {
			return "", nil
		},
	}
}

func TestResolveProfilePackages_NilStore(t *testing.T) {
	deps := createTestDependencies(t)
	deps.ProfileStore = nil

	_, _, err := deps.ResolveProfilePackages("base-dev")
	if err == nil {
		t.Fatal("expected error when ProfileStore is nil")
	}
}

func TestSearchPackages(t *testing.T) {
	deps := createTestDependencies(t)

	results, err := deps.SearchPackages(context.Background(), "git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestListManagedPackages_Empty(t *testing.T) {
	deps := createTestDependencies(t)

	managed, err := deps.ListManagedPackages()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(managed) != 0 {
		t.Fatalf("expected 0 managed packages, got %d", len(managed))
	}
}

func TestRemovePackages_NotManaged(t *testing.T) {
	deps := createTestDependencies(t)

	result, err := deps.RemovePackages(context.Background(), []string{"unknown-pkg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.NotManaged) != 1 {
		t.Fatalf("expected 1 not-managed package, got %d", len(result.NotManaged))
	}

	if result.NotManaged[0] != "unknown-pkg" {
		t.Fatalf("expected 'unknown-pkg' in not-managed, got '%s'", result.NotManaged[0])
	}
}

func TestRemovePackages_DryRun(t *testing.T) {
	deps := createTestDependencies(t)
	deps.DryRun = true

	// Record a package in state first
	deps.State.RecordInstall("git", "test", "apt", true)

	result, err := deps.RemovePackages(context.Background(), []string{"git"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.ToRemove) != 1 {
		t.Fatalf("expected 1 to-remove, got %d", len(result.ToRemove))
	}

	if len(result.PackageResults) != 1 {
		t.Fatalf("expected 1 package result, got %d", len(result.PackageResults))
	}

	if !result.PackageResults[0].Skipped {
		t.Fatal("expected dry-run result to be skipped")
	}
}

func TestUpdatePackages_DryRun(t *testing.T) {
	deps := createTestDependencies(t)
	deps.DryRun = true

	result, err := deps.UpdatePackages(context.Background(), []string{"git", "curl"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(result.Packages))
	}
}

func TestUpdatePackages_Success(t *testing.T) {
	deps := createTestDependencies(t)

	result, err := deps.UpdatePackages(context.Background(), []string{"git"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Packages) != 1 {
		t.Fatalf("expected 1 package, got %d", len(result.Packages))
	}

	if result.Packages[0] != "git" {
		t.Fatalf("expected 'git', got '%s'", result.Packages[0])
	}
}

func TestValidateProfileBytes_Valid(t *testing.T) {
	deps := createTestDependencies(t)

	validYAML := `
name: test-profile
version: "1.0.0"
description: Test profile
author: Test
targets:
  - family: debian
    packages:
      - git
      - curl
env:
  NEXUS_PROFILE: test
`
	profile, err := deps.ValidateProfileBytes([]byte(validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profile.Name != "test-profile" {
		t.Fatalf("expected name 'test-profile', got '%s'", profile.Name)
	}
}

func TestValidateProfileBytes_Invalid(t *testing.T) {
	deps := createTestDependencies(t)

	invalidYAML := `
name: ""
version: ""
`
	_, err := deps.ValidateProfileBytes([]byte(invalidYAML))
	if err == nil {
		t.Fatal("expected error for invalid profile")
	}
}

func TestApplyProfile_NilStore(t *testing.T) {
	deps := createTestDependencies(t)
	deps.ProfileStore = nil

	_, _, err := deps.ApplyProfile(context.Background(), "base-dev")
	if err == nil {
		t.Fatal("expected error when ProfileStore is nil")
	}
}

func TestInstallPackages_DryRun(t *testing.T) {
	deps := createTestDependencies(t)
	deps.DryRun = true

	result, _, err := deps.InstallPackages(context.Background(), []string{"git", "curl"}, "test-profile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// In dry-run mode, the orchestrator should report succeeded packages
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestRemovePackages_MixedManagedAndUnmanaged(t *testing.T) {
	deps := createTestDependencies(t)

	// Record some packages in state
	deps.State.RecordInstall("git", "test", "apt", true)
	deps.State.RecordInstall("curl", "test", "apt", true)

	// Remove one managed and one unmanaged
	result, err := deps.RemovePackages(context.Background(), []string{"git", "unknown-pkg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.ToRemove) != 1 {
		t.Fatalf("expected 1 to-remove, got %d", len(result.ToRemove))
	}

	if len(result.NotManaged) != 1 {
		t.Fatalf("expected 1 not-managed, got %d", len(result.NotManaged))
	}
}

func TestRemovePackages_DependencyWarnings(t *testing.T) {
	deps := createTestDependencies(t)

	// Record foundation and language packages
	deps.State.RecordInstall("ca-certificates", "test", "apt", true)
	deps.State.RecordInstall("python3", "test", "apt", true)

	// Removing a foundation package should warn about dependent language packages
	result, err := deps.RemovePackages(context.Background(), []string{"ca-certificates"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have dependency warnings
	if len(result.DependencyWarnings) == 0 {
		t.Fatal("expected dependency warnings when removing foundation package")
	}

	found := false
	for _, w := range result.DependencyWarnings {
		if contains(w, "python3") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected warning about python3, got warnings: %v", result.DependencyWarnings)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[0] == substr[0] && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestInstallPackages_WithError(t *testing.T) {
	deps := createTestDependencies(t)
	deps.PM = &mockPackageManager{
		name: "apt",
		installFn: func(ctx context.Context, packages []string) ([]installer.PackageResult, error) {
			return nil, fmt.Errorf("package not found")
		},
	}

	_, _, err := deps.InstallPackages(context.Background(), []string{"nonexistent"}, "test")
	if err == nil {
		t.Fatal("expected error from install")
	}
}
