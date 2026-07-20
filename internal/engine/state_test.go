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

package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// newStateTrackerForTest creates a StateTracker with a temp directory.
// This avoids writing to the real ~/.nexus/state.json.
// ---------------------------------------------------------------------------

func newStateTrackerForTest(t *testing.T) *StateTracker {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "state.json")

	tracker := &StateTracker{
		path: path,
		state: &NexusState{
			Version:         1,
			LastModified:    time.Now().UTC(),
			Packages:        make(map[string]PackageState),
			ProfilesApplied: []string{},
			WSLInstances:    make(map[string]WSLInstanceState),
		},
	}

	return tracker
}

// ---------------------------------------------------------------------------
// NewStateTracker — creation and initialization
// ---------------------------------------------------------------------------

func TestNewStateTracker(t *testing.T) {
	// Set HOME to a temp directory so we don't pollute the real home
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	tracker, err := NewStateTracker()
	if err != nil {
		t.Fatalf("NewStateTracker failed: %v", err)
	}
	if tracker == nil {
		t.Fatal("NewStateTracker returned nil")
	}
	if tracker.state == nil {
		t.Fatal("state should not be nil")
	}
	if tracker.state.Version != 1 {
		t.Errorf("Version = %d, want 1", tracker.state.Version)
	}
	if tracker.state.Packages == nil {
		t.Error("Packages map should be initialized")
	}
	if tracker.state.WSLInstances == nil {
		t.Error("WSLInstances map should be initialized")
	}
}

func TestNewStateTracker_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	_, err := NewStateTracker()
	if err != nil {
		t.Fatalf("NewStateTracker failed: %v", err)
	}

	nexusDir := filepath.Join(tmpDir, ".nexus")
	info, err := os.Stat(nexusDir)
	if err != nil {
		t.Fatalf("expected ~/.nexus directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("~/.nexus should be a directory")
	}
}

func TestNewStateTracker_LoadsExisting(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	nexusDir := filepath.Join(tmpDir, ".nexus")
	os.MkdirAll(nexusDir, 0755)

	// Write an existing state file
	existing := &NexusState{
		Version:      1,
		LastModified: time.Now().UTC(),
		Packages: map[string]PackageState{
			"vim": {
				InstalledAt:    time.Now().UTC(),
				Profile:        "base-dev",
				Verified:       true,
				PackageManager: "apt",
			},
		},
		ProfilesApplied: []string{"base-dev"},
		WSLInstances:    make(map[string]WSLInstanceState),
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(nexusDir, "state.json"), data, 0644)

	tracker, err := NewStateTracker()
	if err != nil {
		t.Fatalf("NewStateTracker failed: %v", err)
	}

	// Should load the existing package
	if !tracker.IsManaged("vim") {
		t.Error("should have loaded 'vim' from existing state file")
	}
}

// ---------------------------------------------------------------------------
// RecordInstall and IsManaged
// ---------------------------------------------------------------------------

func TestStateTracker_RecordInstall(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	err := tracker.RecordInstall("git", "base-dev", "apt", true)
	if err != nil {
		t.Fatalf("RecordInstall failed: %v", err)
	}

	if !tracker.IsManaged("git") {
		t.Error("git should be managed after RecordInstall")
	}
}

func TestStateTracker_RecordInstall_MultiplePackages(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	packages := []struct {
		pkg    string
		prof   string
		pm     string
		verify bool
	}{
		{"git", "base-dev", "apt", true},
		{"curl", "base-dev", "apt", true},
		{"vim", "editor", "apt", true},
		{"htop", "monitoring", "pacman", false},
	}

	for _, p := range packages {
		err := tracker.RecordInstall(p.pkg, p.prof, p.pm, p.verify)
		if err != nil {
			t.Fatalf("RecordInstall(%q) failed: %v", p.pkg, err)
		}
	}

	for _, p := range packages {
		if !tracker.IsManaged(p.pkg) {
			t.Errorf("%q should be managed", p.pkg)
		}
	}
}

func TestStateTracker_RecordInstall_SamePackageOverwrites(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	_ = tracker.RecordInstall("git", "base-dev", "apt", false)
	_ = tracker.RecordInstall("git", "full-desktop", "pacman", true)

	pkgs := tracker.GetManagedPackages()
	if pkgs["git"].Profile != "full-desktop" {
		t.Errorf("Profile = %q, want %q", pkgs["git"].Profile, "full-desktop")
	}
	if pkgs["git"].PackageManager != "pacman" {
		t.Errorf("PackageManager = %q, want %q", pkgs["git"].PackageManager, "pacman")
	}
}

func TestStateTracker_IsManaged_NotInstalled(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	if tracker.IsManaged("nonexistent") {
		t.Error("nonexistent package should not be managed")
	}
}

// ---------------------------------------------------------------------------
// RecordRemove
// ---------------------------------------------------------------------------

func TestStateTracker_RecordRemove(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	_ = tracker.RecordInstall("git", "base-dev", "apt", true)
	if !tracker.IsManaged("git") {
		t.Fatal("git should be managed before removal")
	}

	err := tracker.RecordRemove("git")
	if err != nil {
		t.Fatalf("RecordRemove failed: %v", err)
	}

	if tracker.IsManaged("git") {
		t.Error("git should not be managed after RecordRemove")
	}
}

func TestStateTracker_RecordRemove_NonExistent(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	// Removing a package that was never installed should not error
	err := tracker.RecordRemove("ghost")
	if err != nil {
		t.Fatalf("RecordRemove of non-existent package should not error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetManagedPackages
// ---------------------------------------------------------------------------

func TestStateTracker_GetManagedPackages(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	_ = tracker.RecordInstall("git", "base-dev", "apt", true)
	_ = tracker.RecordInstall("curl", "base-dev", "apt", true)

	pkgs := tracker.GetManagedPackages()
	if len(pkgs) != 2 {
		t.Errorf("GetManagedPackages returned %d packages, want 2", len(pkgs))
	}

	if _, ok := pkgs["git"]; !ok {
		t.Error("expected 'git' in managed packages")
	}
	if _, ok := pkgs["curl"]; !ok {
		t.Error("expected 'curl' in managed packages")
	}
}

func TestStateTracker_GetManagedPackages_ReturnsCopy(t *testing.T) {
	tracker := newStateTrackerForTest(t)
	_ = tracker.RecordInstall("git", "base-dev", "apt", true)

	pkgs := tracker.GetManagedPackages()
	// Mutating the returned map should not affect the tracker
	pkgs["injected"] = PackageState{}

	if tracker.IsManaged("injected") {
		t.Error("GetManagedPackages should return a copy, not a reference")
	}
}

// ---------------------------------------------------------------------------
// GetProfiles / RecordInstall auto-tracks profiles
// ---------------------------------------------------------------------------

func TestStateTracker_GetProfiles(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	_ = tracker.RecordInstall("git", "base-dev", "apt", true)
	_ = tracker.RecordInstall("vim", "editor", "apt", true)

	profiles := tracker.GetProfiles()
	if len(profiles) != 2 {
		t.Errorf("GetProfiles returned %d profiles, want 2", len(profiles))
	}

	hasBaseDev := false
	hasEditor := false
	for _, p := range profiles {
		if p == "base-dev" {
			hasBaseDev = true
		}
		if p == "editor" {
			hasEditor = true
		}
	}
	if !hasBaseDev {
		t.Error("expected 'base-dev' profile")
	}
	if !hasEditor {
		t.Error("expected 'editor' profile")
	}
}

func TestStateTracker_ProfilesNoDuplicate(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	_ = tracker.RecordInstall("git", "base-dev", "apt", true)
	_ = tracker.RecordInstall("curl", "base-dev", "apt", true)

	profiles := tracker.GetProfiles()
	count := 0
	for _, p := range profiles {
		if p == "base-dev" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("base-dev profile appeared %d times, want 1", count)
	}
}

// ---------------------------------------------------------------------------
// WSL State: RecordWSLImport, RecordWSLRemove, GetWSLInstances, IsWSLManaged
// ---------------------------------------------------------------------------

func TestStateTracker_RecordWSLImport(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	err := tracker.RecordWSLImport(
		"ubuntu-22.04", "Ubuntu", "22.04",
		"sha256:abc123", "/home/user/.nexus/wsl/ubuntu-22.04", "debian",
	)
	if err != nil {
		t.Fatalf("RecordWSLImport failed: %v", err)
	}

	if !tracker.IsWSLManaged("ubuntu-22.04") {
		t.Error("ubuntu-22.04 should be WSL managed after import")
	}
}

func TestStateTracker_RecordWSLImport_Multiple(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	instances := []struct {
		name    string
		image   string
		version string
		sha256  string
		path    string
		family  string
	}{
		{"ubuntu-22.04", "Ubuntu", "22.04", "sha256:abc", "/path/ubuntu", "debian"},
		{"arch-wsl", "Arch", "latest", "sha256:def", "/path/arch", "arch"},
		{"fedora-39", "Fedora", "39", "sha256:ghi", "/path/fedora", "redhat"},
	}

	for _, inst := range instances {
		err := tracker.RecordWSLImport(inst.name, inst.image, inst.version, inst.sha256, inst.path, inst.family)
		if err != nil {
			t.Fatalf("RecordWSLImport(%q) failed: %v", inst.name, err)
		}
	}

	instances_map := tracker.GetWSLInstances()
	if len(instances_map) != 3 {
		t.Errorf("GetWSLInstances returned %d instances, want 3", len(instances_map))
	}

	// Verify specific instance data
	ubuntu := instances_map["ubuntu-22.04"]
	if ubuntu.ImageName != "Ubuntu" {
		t.Errorf("ImageName = %q, want %q", ubuntu.ImageName, "Ubuntu")
	}
	if ubuntu.Family != "debian" {
		t.Errorf("Family = %q, want %q", ubuntu.Family, "debian")
	}
	if ubuntu.TarballSHA256 != "sha256:abc" {
		t.Errorf("TarballSHA256 = %q, want %q", ubuntu.TarballSHA256, "sha256:abc")
	}
}

func TestStateTracker_RecordWSLRemove(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	_ = tracker.RecordWSLImport("ubuntu-22.04", "Ubuntu", "22.04", "sha256:abc", "/path", "debian")
	if !tracker.IsWSLManaged("ubuntu-22.04") {
		t.Fatal("should be managed before removal")
	}

	err := tracker.RecordWSLRemove("ubuntu-22.04")
	if err != nil {
		t.Fatalf("RecordWSLRemove failed: %v", err)
	}

	if tracker.IsWSLManaged("ubuntu-22.04") {
		t.Error("ubuntu-22.04 should not be managed after RecordWSLRemove")
	}
}

func TestStateTracker_RecordWSLRemove_NonExistent(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	err := tracker.RecordWSLRemove("ghost-distro")
	if err != nil {
		t.Fatalf("RecordWSLRemove of non-existent should not error: %v", err)
	}
}

func TestStateTracker_GetWSLInstances_ReturnsCopy(t *testing.T) {
	tracker := newStateTrackerForTest(t)
	_ = tracker.RecordWSLImport("ubuntu", "Ubuntu", "22.04", "sha256:abc", "/path", "debian")

	instances := tracker.GetWSLInstances()
	instances["injected"] = WSLInstanceState{}

	if tracker.IsWSLManaged("injected") {
		t.Error("GetWSLInstances should return a copy, not a reference")
	}
}

func TestStateTracker_IsWSLManaged_NotImported(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	if tracker.IsWSLManaged("nonexistent") {
		t.Error("nonexistent instance should not be managed")
	}
}

// ---------------------------------------------------------------------------
// Persistence — state is written to disk correctly
// ---------------------------------------------------------------------------

func TestStateTracker_PersistsToDisk(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	_ = tracker.RecordInstall("git", "base-dev", "apt", true)

	// Verify the state file was written
	data, err := os.ReadFile(tracker.path)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	var loaded NexusState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal state: %v", err)
	}

	if _, ok := loaded.Packages["git"]; !ok {
		t.Error("persisted state should contain 'git'")
	}
}

func TestStateTracker_AtomicWrite(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	_ = tracker.RecordInstall("git", "base-dev", "apt", true)

	// The .tmp file should not exist after a successful save
	tmpPath := tracker.path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after atomic rename")
	}
}

// ---------------------------------------------------------------------------
// Concurrent access safety
// ---------------------------------------------------------------------------

func TestStateTracker_ConcurrentAccess(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	var wg sync.WaitGroup
	numOps := 50

	// Concurrent writes
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			pkg := "pkg-" + string(rune('a'+idx%26))
			_ = tracker.RecordInstall(pkg, "concurrent-test", "apt", true)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			pkg := "pkg-" + string(rune('a'+idx%26))
			_ = tracker.IsManaged(pkg)
			_ = tracker.GetManagedPackages()
			_ = tracker.GetProfiles()
		}(i)
	}

	// Concurrent WSL operations
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := "wsl-" + string(rune('a'+idx%26))
			_ = tracker.RecordWSLImport(name, "Test", "1.0", "sha256:test", "/path", "debian")
			_ = tracker.IsWSLManaged(name)
		}(i)
	}

	wg.Wait()

	// Verify state is still valid
	pkgs := tracker.GetManagedPackages()
	if len(pkgs) == 0 {
		t.Error("should have at least some packages after concurrent writes")
	}
}

// ---------------------------------------------------------------------------
// NexusState — JSON serialization
// ---------------------------------------------------------------------------

func TestNexusState_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	state := &NexusState{
		Version:      1,
		LastModified: time.Now().UTC().Truncate(time.Millisecond),
		Packages: map[string]PackageState{
			"git": {
				InstalledAt:    time.Now().UTC().Truncate(time.Millisecond),
				Profile:        "base-dev",
				Verified:       true,
				PackageManager: "apt",
			},
		},
		ProfilesApplied: []string{"base-dev"},
		WSLInstances: map[string]WSLInstanceState{
			"ubuntu": {
				ImageName:     "Ubuntu",
				ImageVersion:  "22.04",
				TarballSHA256: "sha256:abc123",
				InstallPath:   "/path/to/ubuntu",
				Family:        "debian",
				ImportedAt:    time.Now().UTC().Truncate(time.Millisecond),
			},
		},
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded NexusState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Version != state.Version {
		t.Errorf("Version: got %d, want %d", decoded.Version, state.Version)
	}
	if len(decoded.Packages) != 1 {
		t.Errorf("Packages count: got %d, want 1", len(decoded.Packages))
	}
	if len(decoded.WSLInstances) != 1 {
		t.Errorf("WSLInstances count: got %d, want 1", len(decoded.WSLInstances))
	}
}

func TestPackageState_Fields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	ps := PackageState{
		InstalledAt:    now,
		Profile:        "base-dev",
		Verified:       true,
		PackageManager: "apt",
	}

	data, _ := json.Marshal(ps)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	expectedFields := []string{"installed_at", "profile", "verified", "package_manager"}
	for _, f := range expectedFields {
		if _, ok := raw[f]; !ok {
			t.Errorf("PackageState JSON missing field %q", f)
		}
	}
}

func TestWSLInstanceState_Fields(t *testing.T) {
	t.Parallel()

	ws := WSLInstanceState{
		ImageName:     "Ubuntu",
		ImageVersion:  "22.04",
		TarballSHA256: "sha256:abc",
		InstallPath:   "/path",
		Family:        "debian",
		ImportedAt:    time.Now().UTC(),
	}

	data, _ := json.Marshal(ws)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	expectedFields := []string{
		"image_name", "image_version", "tarball_sha256",
		"install_path", "family", "imported_at",
	}
	for _, f := range expectedFields {
		if _, ok := raw[f]; !ok {
			t.Errorf("WSLInstanceState JSON missing field %q", f)
		}
	}
}

// ---------------------------------------------------------------------------
// save — error path tests
// ---------------------------------------------------------------------------

// TestStateTracker_Save_WriteFileError verifies that save returns an error
// when the path's parent directory does not exist (WriteFile fails).
func TestStateTracker_Save_WriteFileError(t *testing.T) {
	tracker := &StateTracker{
		path: "/nonexistent/deeply/nested/dir/state.json",
		state: &NexusState{
			Version:         1,
			LastModified:    time.Now().UTC(),
			Packages:        make(map[string]PackageState),
			ProfilesApplied: []string{},
			WSLInstances:    make(map[string]WSLInstanceState),
		},
	}

	err := tracker.save()
	if err == nil {
		t.Fatal("save should return an error when parent directory does not exist")
	}
	if !strings.Contains(err.Error(), "failed to write state") {
		t.Errorf("error should mention 'failed to write state', got: %v", err)
	}
}

// TestStateTracker_Save_RenameError verifies that save returns an error
// when the rename step fails. We create a scenario where the temp file
// can be written but the rename target is invalid (e.g., the target path
// is a directory instead of a file).
func TestStateTracker_Save_RenameError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory at the target path — rename to a directory fails
	targetDir := filepath.Join(tmpDir, "state.json")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("failed to create target dir: %v", err)
	}

	tracker := &StateTracker{
		path: filepath.Join(tmpDir, "state.json"),
		state: &NexusState{
			Version:         1,
			LastModified:    time.Now().UTC(),
			Packages:        make(map[string]PackageState),
			ProfilesApplied: []string{},
			WSLInstances:    make(map[string]WSLInstanceState),
		},
	}

	err := tracker.save()
	if err == nil {
		t.Fatal("save should return an error when rename fails")
	}
	if !strings.Contains(err.Error(), "failed to commit state") {
		t.Errorf("error should mention 'failed to commit state', got: %v", err)
	}

	// The temp file should be cleaned up after rename failure
	tmpPath := tracker.path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should be cleaned up after rename failure")
	}
}

// TestStateTracker_RecordInstall_SaveError verifies that RecordInstall
// propagates save errors correctly.
func TestStateTracker_RecordInstall_SaveError(t *testing.T) {
	tracker := &StateTracker{
		path: "/nonexistent/deeply/nested/dir/state.json",
		state: &NexusState{
			Version:         1,
			LastModified:    time.Now().UTC(),
			Packages:        make(map[string]PackageState),
			ProfilesApplied: []string{},
			WSLInstances:    make(map[string]WSLInstanceState),
		},
	}

	err := tracker.RecordInstall("git", "dev", "apt", true)
	if err == nil {
		t.Fatal("RecordInstall should return an error when save fails")
	}
	if !strings.Contains(err.Error(), "failed to write state") {
		t.Errorf("error should mention 'failed to write state', got: %v", err)
	}
}

// TestStateTracker_RecordRemove_SaveError verifies that RecordRemove
// propagates save errors correctly.
func TestStateTracker_RecordRemove_SaveError(t *testing.T) {
	tracker := &StateTracker{
		path: "/nonexistent/deeply/nested/dir/state.json",
		state: &NexusState{
			Version:         1,
			LastModified:    time.Now().UTC(),
			Packages:        make(map[string]PackageState),
			ProfilesApplied: []string{},
			WSLInstances:    make(map[string]WSLInstanceState),
		},
	}

	err := tracker.RecordRemove("git")
	if err == nil {
		t.Fatal("RecordRemove should return an error when save fails")
	}
	if !strings.Contains(err.Error(), "failed to write state") {
		t.Errorf("error should mention 'failed to write state', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// V7: Dotfiles state methods
// ---------------------------------------------------------------------------

func TestStateTracker_RecordDotfilesInstall(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	if err := tracker.RecordDotfilesInstall("2.51.0"); err != nil {
		t.Fatalf("RecordDotfilesInstall failed: %v", err)
	}

	if !tracker.IsDotfilesInstalled() {
		t.Error("dotfiles should be installed after RecordDotfilesInstall")
	}

	st := tracker.GetDotfilesState()
	if st.Version != "2.51.0" {
		t.Errorf("Version = %q, want %q", st.Version, "2.51.0")
	}
	if st.InstalledAt.IsZero() {
		t.Error("InstalledAt should be set")
	}
}

func TestStateTracker_GetDotfilesState_Empty(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	st := tracker.GetDotfilesState()
	if st.Installed {
		t.Error("Installed should be false on empty state")
	}
	if st.Source != "" {
		t.Errorf("Source should be empty, got %q", st.Source)
	}
}

func TestStateTracker_IsDotfilesInstalled_Default(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	if tracker.IsDotfilesInstalled() {
		t.Error("IsDotfilesInstalled should be false by default")
	}
}

func TestStateTracker_RecordDotfilesInit(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	if err := tracker.RecordDotfilesInstall("2.51.0"); err != nil {
		t.Fatalf("setup RecordDotfilesInstall failed: %v", err)
	}

	if err := tracker.RecordDotfilesInit("https://github.com/user/dots.git"); err != nil {
		t.Fatalf("RecordDotfilesInit failed: %v", err)
	}

	st := tracker.GetDotfilesState()
	if st.Source != "https://github.com/user/dots.git" {
		t.Errorf("Source = %q, want %q", st.Source, "https://github.com/user/dots.git")
	}
	if st.InitializedAt.IsZero() {
		t.Error("InitializedAt should be set")
	}
	// RecordDotfilesInit should preserve Version and InstalledAt
	if st.Version != "2.51.0" {
		t.Errorf("Version should be preserved, got %q", st.Version)
	}
}

func TestStateTracker_RecordDotfilesApply(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	managedFiles := []string{"/home/user/.zshrc", "/home/user/.gitconfig"}
	if err := tracker.RecordDotfilesApply(managedFiles); err != nil {
		t.Fatalf("RecordDotfilesApply failed: %v", err)
	}

	st := tracker.GetDotfilesState()
	if st.LastAppliedAt.IsZero() {
		t.Error("LastAppliedAt should be set")
	}
	if len(st.ManagedFiles) != 2 {
		t.Errorf("ManagedFiles len = %d, want 2", len(st.ManagedFiles))
	}
}

func TestStateTracker_RecordDotfilesAdd(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	if err := tracker.RecordDotfilesAdd("/home/user/.zshrc"); err != nil {
		t.Fatalf("RecordDotfilesAdd failed: %v", err)
	}

	st := tracker.GetDotfilesState()
	if len(st.ManagedFiles) != 1 || st.ManagedFiles[0] != "/home/user/.zshrc" {
		t.Errorf("ManagedFiles = %v, want [/home/user/.zshrc]", st.ManagedFiles)
	}
}

func TestStateTracker_RecordDotfilesAdd_Idempotent(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	// Adding the same path twice should not create a duplicate.
	_ = tracker.RecordDotfilesAdd("/home/user/.zshrc")
	if err := tracker.RecordDotfilesAdd("/home/user/.zshrc"); err != nil {
		t.Fatalf("second RecordDotfilesAdd failed: %v", err)
	}

	st := tracker.GetDotfilesState()
	if len(st.ManagedFiles) != 1 {
		t.Errorf("ManagedFiles should remain deduped, got %d entries: %v", len(st.ManagedFiles), st.ManagedFiles)
	}
}

func TestStateTracker_RecordDotfilesRemove(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	_ = tracker.RecordDotfilesInit("https://github.com/user/dots.git")

	if err := tracker.RecordDotfilesRemove(); err != nil {
		t.Fatalf("RecordDotfilesRemove failed: %v", err)
	}

	st := tracker.GetDotfilesState()
	if st.Source != "" {
		t.Errorf("Source should be empty after remove, got %q", st.Source)
	}
	if !st.InitializedAt.IsZero() {
		t.Error("InitializedAt should be zero after remove")
	}
}

// ---------------------------------------------------------------------------
// V8: Git-sync state methods
// ---------------------------------------------------------------------------

func TestStateTracker_RecordDotfilesPush(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	sha := "abc1234567890def"
	if err := tracker.RecordDotfilesPush(sha); err != nil {
		t.Fatalf("RecordDotfilesPush failed: %v", err)
	}

	status := tracker.GetDotfilesSyncStatus()
	if status.LastCommitSHA != sha {
		t.Errorf("LastCommitSHA = %q, want %q", status.LastCommitSHA, sha)
	}
	if status.LastPushedAt.IsZero() {
		t.Error("LastPushedAt should be set")
	}
}

func TestStateTracker_RecordDotfilesPush_EmptySHA(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	// Push with a real SHA, then push again with empty — LastCommitSHA
	// should NOT be cleared (we only update the SHA on non-empty pushes).
	_ = tracker.RecordDotfilesPush("real-sha")
	if err := tracker.RecordDotfilesPush(""); err != nil {
		t.Fatalf("RecordDotfilesPush empty failed: %v", err)
	}

	status := tracker.GetDotfilesSyncStatus()
	if status.LastCommitSHA != "real-sha" {
		t.Errorf("LastCommitSHA should be preserved when pushing with empty SHA, got %q", status.LastCommitSHA)
	}
}

func TestStateTracker_RecordDotfilesPull(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	sha := "pulled-sha-1234"
	if err := tracker.RecordDotfilesPull(sha); err != nil {
		t.Fatalf("RecordDotfilesPull failed: %v", err)
	}

	status := tracker.GetDotfilesSyncStatus()
	if status.LastCommitSHA != sha {
		t.Errorf("LastCommitSHA = %q, want %q", status.LastCommitSHA, sha)
	}
	if status.LastPulledAt.IsZero() {
		t.Error("LastPulledAt should be set")
	}
}

func TestStateTracker_GetDotfilesSyncStatus_Empty(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	status := tracker.GetDotfilesSyncStatus()
	if status.LastCommitSHA != "" {
		t.Errorf("LastCommitSHA should be empty, got %q", status.LastCommitSHA)
	}
	if !status.LastPushedAt.IsZero() {
		t.Error("LastPushedAt should be zero on empty state")
	}
	if !status.LastPulledAt.IsZero() {
		t.Error("LastPulledAt should be zero on empty state")
	}
}

// ---------------------------------------------------------------------------
// V9: Vault state methods
// ---------------------------------------------------------------------------

func TestStateTracker_RecordVaultInit(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	pubKey := "age1ql3z7hjy54tw3uv8lwml0vqcpdv82z0vk9e6j4den6ghe6q0v3fq0f3s7l"
	keyPath := "/home/user/.nexus/vault/private.key"
	keyringID := "nexus-dotfiles-vault"

	if err := tracker.RecordVaultInit(pubKey, keyPath, keyringID); err != nil {
		t.Fatalf("RecordVaultInit failed: %v", err)
	}

	v := tracker.GetVaultState()
	if !v.Initialized {
		t.Error("Vault should be marked initialized")
	}
	if v.PublicKey != pubKey {
		t.Errorf("PublicKey = %q, want %q", v.PublicKey, pubKey)
	}
	if v.PublicKeyShort != ShortKeyFingerprint(pubKey) {
		t.Errorf("PublicKeyShort = %q, want %q", v.PublicKeyShort, ShortKeyFingerprint(pubKey))
	}
	if v.KeyPath != keyPath {
		t.Errorf("KeyPath = %q, want %q", v.KeyPath, keyPath)
	}
	if v.KeyringID != keyringID {
		t.Errorf("KeyringID = %q, want %q", v.KeyringID, keyringID)
	}
	if v.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if v.EncryptedFiles == nil {
		t.Error("EncryptedFiles map should be initialized (not nil)")
	}
}

func TestStateTracker_RecordVaultAdd(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	// Init first so the map is initialized.
	_ = tracker.RecordVaultInit("age1pubkey", "/path/to/key", "nexus-dotfiles-vault")

	original := "/home/user/.ssh/id_ed25519"
	encrypted := "/home/user/.local/share/chezmoi/id_ed25519.age"

	if err := tracker.RecordVaultAdd(original, encrypted); err != nil {
		t.Fatalf("RecordVaultAdd failed: %v", err)
	}

	v := tracker.GetVaultState()
	if v.EncryptedFiles[original] != encrypted {
		t.Errorf("EncryptedFiles[%q] = %q, want %q", original, v.EncryptedFiles[original], encrypted)
	}
}

func TestStateTracker_RecordVaultAdd_InitializesMap(t *testing.T) {
	// Tracker's Vault.EncryptedFiles starts as nil — RecordVaultAdd should
	// initialize it on demand (mirrors RecordVaultInit behavior).
	tracker := newStateTrackerForTest(t)

	if err := tracker.RecordVaultAdd("/some/file", "/some/file.age"); err != nil {
		t.Fatalf("RecordVaultAdd should succeed even without prior init: %v", err)
	}

	v := tracker.GetVaultState()
	if v.EncryptedFiles == nil {
		t.Fatal("EncryptedFiles should be initialized by RecordVaultAdd")
	}
	if v.EncryptedFiles["/some/file"] != "/some/file.age" {
		t.Errorf("EncryptedFiles mapping incorrect: %v", v.EncryptedFiles)
	}
}

func TestStateTracker_RecordVaultRemove(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	_ = tracker.RecordVaultInit("age1pubkey", "/path/to/key", "id")
	_ = tracker.RecordVaultAdd("/file/a", "/file/a.age")
	_ = tracker.RecordVaultAdd("/file/b", "/file/b.age")

	if err := tracker.RecordVaultRemove("/file/a"); err != nil {
		t.Fatalf("RecordVaultRemove failed: %v", err)
	}

	v := tracker.GetVaultState()
	if _, ok := v.EncryptedFiles["/file/a"]; ok {
		t.Error("Expected /file/a to be removed from EncryptedFiles")
	}
	if _, ok := v.EncryptedFiles["/file/b"]; !ok {
		t.Error("Expected /file/b to remain in EncryptedFiles")
	}
}

func TestStateTracker_GetVaultState_Empty(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	v := tracker.GetVaultState()
	if v.Initialized {
		t.Error("Vault should not be initialized by default")
	}
	if v.PublicKey != "" {
		t.Errorf("PublicKey should be empty, got %q", v.PublicKey)
	}
}

func TestShortKeyFingerprint(t *testing.T) {
	tests := []struct {
		name string
		pub  string
		want string
	}{
		{"long key truncated", "age1ql3z7hjy54tw3uv8lwml0vqcpdv82z0vk9e6j4den6ghe6q0v3fq0f3s7l", "age1ql3z7hjy54tw"},
		{"exactly 16 chars", "age1ql3z7hjy54tw", "age1ql3z7hjy54tw"},
		{"shorter than 16", "age1short", "age1short"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortKeyFingerprint(tt.pub)
			if got != tt.want {
				t.Errorf("ShortKeyFingerprint(%q) = %q, want %q", tt.pub, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// V13: Hardware Ledger state methods
// ---------------------------------------------------------------------------

func TestStateTracker_RecordLedgerEntry(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	report := HardwareReport{
		DeviceFingerprint: "test-fp-001",
		OS:                "linux",
		Arch:              "amd64",
		CPUModel:          "Test CPU",
		CPUCores:          8,
		RAMTotalMB:        16384,
		GPU:               "Test GPU",
		Success:           true,
		ProfileName:       "test-profile",
		RecordedAt:        time.Now().UTC(),
	}

	if err := tracker.RecordLedgerEntry(report); err != nil {
		t.Fatalf("RecordLedgerEntry failed: %v", err)
	}

	ledger := tracker.GetLedger()
	if len(ledger.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(ledger.Records))
	}
	if ledger.Records[0].DeviceFingerprint != "test-fp-001" {
		t.Errorf("fingerprint = %q, want %q", ledger.Records[0].DeviceFingerprint, "test-fp-001")
	}
	if ledger.LastAnalyzedAt.IsZero() {
		t.Error("LastAnalyzedAt should be set after RecordLedgerEntry")
	}
}

func TestStateTracker_RecordLedgerEntry_BoundedRing(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	for i := 0; i < MaxLedgerRecords+5; i++ {
		report := HardwareReport{
			DeviceFingerprint: "fp",
			OS:                "linux",
			CPUModel:          "CPU",
			CPUCores:          i + 1,
			Success:           true,
			RecordedAt:        time.Now().UTC(),
		}
		if err := tracker.RecordLedgerEntry(report); err != nil {
			t.Fatalf("RecordLedgerEntry %d failed: %v", i, err)
		}
	}

	ledger := tracker.GetLedger()
	if len(ledger.Records) != MaxLedgerRecords {
		t.Errorf("expected %d records, got %d", MaxLedgerRecords, len(ledger.Records))
	}

	// The first 5 should have been dropped, so the first record should have CPUCores=6
	if ledger.Records[0].CPUCores != 6 {
		t.Errorf("expected first record CPUCores=6 after dropping 5, got %d", ledger.Records[0].CPUCores)
	}
}

func TestStateTracker_GetLedger_Empty(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	ledger := tracker.GetLedger()
	if len(ledger.Records) != 0 {
		t.Errorf("expected 0 records, got %d", len(ledger.Records))
	}
	if ledger.CommunitySyncEnabled {
		t.Error("CommunitySyncEnabled should be false by default")
	}
}

func TestStateTracker_GetLedger_ReturnsCopy(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	_ = tracker.RecordLedgerEntry(HardwareReport{
		DeviceFingerprint: "fp-1",
		OS:                "linux",
		Success:           true,
		RecordedAt:        time.Now().UTC(),
	})

	ledger := tracker.GetLedger()
	ledger.Records[0].OS = "mutated"
	ledger.Records = append(ledger.Records, HardwareReport{})

	// Verify the original is unaffected
	ledger2 := tracker.GetLedger()
	if len(ledger2.Records) != 1 {
		t.Error("GetLedger should return a copy")
	}
	if ledger2.Records[0].OS != "linux" {
		t.Error("GetLedger should return a copy")
	}
}

func TestStateTracker_SetCommunitySyncEnabled(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	if err := tracker.SetCommunitySyncEnabled(true); err != nil {
		t.Fatalf("SetCommunitySyncEnabled(true): %v", err)
	}

	ledger := tracker.GetLedger()
	if !ledger.CommunitySyncEnabled {
		t.Error("expected CommunitySyncEnabled=true")
	}

	if err := tracker.SetCommunitySyncEnabled(false); err != nil {
		t.Fatalf("SetCommunitySyncEnabled(false): %v", err)
	}

	ledger = tracker.GetLedger()
	if ledger.CommunitySyncEnabled {
		t.Error("expected CommunitySyncEnabled=false after disabling")
	}
}

func TestStateTracker_RecordLedgerSync(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	if err := tracker.RecordLedgerSync(); err != nil {
		t.Fatalf("RecordLedgerSync failed: %v", err)
	}

	ledger := tracker.GetLedger()
	if ledger.LastSyncedAt.IsZero() {
		t.Error("LastSyncedAt should be set after RecordLedgerSync")
	}
}

func TestStateTracker_RecordLedgerEntry_SaveError(t *testing.T) {
	tracker := &StateTracker{
		path: "/nonexistent/deeply/nested/dir/state.json",
		state: &NexusState{
			Version:         1,
			LastModified:    time.Now().UTC(),
			Packages:        make(map[string]PackageState),
			ProfilesApplied: []string{},
			WSLInstances:    make(map[string]WSLInstanceState),
		},
	}

	err := tracker.RecordLedgerEntry(HardwareReport{
		DeviceFingerprint: "fp",
		OS:                "linux",
		Success:           true,
		RecordedAt:        time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("RecordLedgerEntry should return an error when save fails")
	}
	if !strings.Contains(err.Error(), "failed to write state") {
		t.Errorf("error should mention 'failed to write state', got: %v", err)
	}
}

func TestStateTracker_SetCommunitySyncEnabled_SaveError(t *testing.T) {
	tracker := &StateTracker{
		path: "/nonexistent/deeply/nested/dir/state.json",
		state: &NexusState{
			Version:         1,
			LastModified:    time.Now().UTC(),
			Packages:        make(map[string]PackageState),
			ProfilesApplied: []string{},
			WSLInstances:    make(map[string]WSLInstanceState),
		},
	}

	err := tracker.SetCommunitySyncEnabled(true)
	if err == nil {
		t.Fatal("SetCommunitySyncEnabled should return an error when save fails")
	}
	if !strings.Contains(err.Error(), "failed to write state") {
		t.Errorf("error should mention 'failed to write state', got: %v", err)
	}
}

func TestStateTracker_RecordLedgerSync_SaveError(t *testing.T) {
	tracker := &StateTracker{
		path: "/nonexistent/deeply/nested/dir/state.json",
		state: &NexusState{
			Version:         1,
			LastModified:    time.Now().UTC(),
			Packages:        make(map[string]PackageState),
			ProfilesApplied: []string{},
			WSLInstances:    make(map[string]WSLInstanceState),
		},
	}

	err := tracker.RecordLedgerSync()
	if err == nil {
		t.Fatal("RecordLedgerSync should return an error when save fails")
	}
	if !strings.Contains(err.Error(), "failed to write state") {
		t.Errorf("error should mention 'failed to write state', got: %v", err)
	}
}

func TestHardwareReport_JSONFields(t *testing.T) {
	t.Parallel()

	hr := HardwareReport{
		DeviceFingerprint: "abc123",
		OS:                "linux",
		Success:           true,
		RecordedAt:        time.Now().UTC(),
	}

	data, _ := json.Marshal(hr)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	expectedFields := []string{
		"device_fingerprint", "os", "arch", "kernel", "cpu_model",
		"cpu_cores", "ram_total_mb", "disk_total_gb", "gpu", "is_wsl2",
		"package_manager", "success", "profile_name", "recorded_at",
	}
	for _, f := range expectedFields {
		if _, ok := raw[f]; !ok {
			t.Errorf("HardwareReport JSON missing field %q", f)
		}
	}
}

func TestHardwareLedger_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Millisecond)

	ledger := HardwareLedger{
		Records: []HardwareReport{
			{
				DeviceFingerprint: "fp-1",
				OS:                "linux",
				Arch:              "amd64",
				CPUModel:          "Intel i7",
				CPUCores:          8,
				RAMTotalMB:        16384,
				DiskTotalGB:       512.0,
				GPU:               "NVIDIA RTX 3070",
				Success:           true,
				ProfileName:       "base-dev",
				RecordedAt:        now,
			},
		},
		LastAnalyzedAt:       now,
		CommunitySyncEnabled: true,
		LastSyncedAt:         now,
	}

	data, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded HardwareLedger
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if len(decoded.Records) != 1 {
		t.Errorf("Records: got %d, want 1", len(decoded.Records))
	}
	if decoded.Records[0].DeviceFingerprint != "fp-1" {
		t.Errorf("fingerprint: got %q, want %q", decoded.Records[0].DeviceFingerprint, "fp-1")
	}
	if decoded.Records[0].OS != "linux" {
		t.Errorf("OS: got %q, want %q", decoded.Records[0].OS, "linux")
	}
	if !decoded.CommunitySyncEnabled {
		t.Error("CommunitySyncEnabled should be preserved")
	}
	if decoded.LastSyncedAt.IsZero() {
		t.Error("LastSyncedAt should be preserved")
	}
}
