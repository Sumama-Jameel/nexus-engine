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
        _ = os.MkdirAll(nexusDir, 0755) //nolint:gosec

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
        _ = os.WriteFile(filepath.Join(nexusDir, "state.json"), data, 0644) //nolint:gosec

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
                                ImageName:      "Ubuntu",
                                ImageVersion:   "22.04",
                                TarballSHA256:  "sha256:abc123",
                                InstallPath:    "/path/to/ubuntu",
                                Family:         "debian",
                                ImportedAt:     time.Now().UTC().Truncate(time.Millisecond),
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
        _ = json.Unmarshal(data, &raw)

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
                ImageName:      "Ubuntu",
                ImageVersion:   "22.04",
                TarballSHA256:  "sha256:abc",
                InstallPath:    "/path",
                Family:         "debian",
                ImportedAt:     time.Now().UTC(),
        }

        data, _ := json.Marshal(ws)
        var raw map[string]interface{}
        _ = json.Unmarshal(data, &raw)

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
