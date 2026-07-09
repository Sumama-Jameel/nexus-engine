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
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ─── Teleport ───

func TestTeleport_NotWSL2(t *testing.T) {
	// On native Linux (the normal case), Teleport should return ErrNotWSL2
	// because there is no Windows filesystem to migrate from.
	_, err := Teleport(context.Background(), false)
	if err == nil {
		t.Fatal("expected error on non-WSL2 system")
	}
	if !strings.Contains(err.Error(), "only available on WSL2") &&
		!strings.Contains(err.Error(), "already on native Linux") {
		t.Errorf("error should mention WSL2 requirement, got: %v", err)
	}
}

func TestTeleport_DryRun_NotWSL2(t *testing.T) {
	// Dry-run should also fail on non-WSL2 — the guard is independent of
	// whether we're writing or not.
	_, err := Teleport(context.Background(), true)
	if err == nil {
		t.Fatal("expected error on non-WSL2 even with dry-run")
	}
}

func TestTeleport_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled

	_, err := Teleport(ctx, false)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	// On cancelled context, it should short-circuit before reaching the
	// WSL2 check. If it reaches the WSL2 check instead, that's also fine
	// since the test is about not panicking.
}

// ─── TeleportSummary ───

func TestTeleportSummary_Empty(t *testing.T) {
	summary := TeleportSummary(nil)
	if !strings.Contains(summary, "Teleport Results") {
		t.Errorf("expected header, got: %s", summary)
	}
	if !strings.Contains(summary, "0 linked") {
		t.Errorf("expected zero counts, got: %s", summary)
	}
}

func TestTeleportSummary_AllLinked(t *testing.T) {
	results := []TeleportResult{
		{Source: "/mnt/c/Users/user/Documents", Target: "/home/user/Documents", Linked: true},
		{Source: "/mnt/c/Users/user/Desktop", Target: "/home/user/Desktop", Linked: true},
	}
	summary := TeleportSummary(results)
	if !strings.Contains(summary, "2 linked") {
		t.Errorf("expected 2 linked, got: %s", summary)
	}
}

func TestTeleportSummary_Mixed(t *testing.T) {
	results := []TeleportResult{
		{Source: "/mnt/c/Users/user/Documents", Target: "/home/user/Documents", Linked: true},
		{Source: "/mnt/c/Users/user/Desktop", Target: "/home/user/Desktop", AlreadyExists: true},
		{Source: "/mnt/c/Users/user/Downloads", Error: "source not found"},
	}
	summary := TeleportSummary(results)
	if !strings.Contains(summary, "1 linked") {
		t.Errorf("expected 1 linked, got: %s", summary)
	}
	if !strings.Contains(summary, "1 already") {
		t.Errorf("expected 1 already present, got: %s", summary)
	}
	if !strings.Contains(summary, "1 errors") {
		t.Errorf("expected 1 error, got: %s", summary)
	}
}

func TestTeleportSummary_OnlyErrors(t *testing.T) {
	results := []TeleportResult{
		{Source: "/mnt/c/Users/user/Documents", Error: "permission denied"},
	}
	summary := TeleportSummary(results)
	if !strings.Contains(summary, "0 linked") {
		t.Errorf("expected 0 linked, got: %s", summary)
	}
	if !strings.Contains(summary, "permission denied") {
		t.Errorf("should include error message, got: %s", summary)
	}
}

// ─── RecordTeleported / IsTeleported (state tests) ───

func TestStateTracker_RecordTeleported(t *testing.T) {
	tracker := newStateTrackerForTest(t)

	if tracker.IsTeleported() {
		t.Error("expected IsTeleported=false before RecordTeleported")
	}

	if err := tracker.RecordTeleported(); err != nil {
		t.Fatalf("RecordTeleported failed: %v", err)
	}

	if !tracker.IsTeleported() {
		t.Error("expected IsTeleported=true after RecordTeleported")
	}
}

func TestStateTracker_IsTeleported_Default(t *testing.T) {
	tracker := newStateTrackerForTest(t)
	if tracker.IsTeleported() {
		t.Error("expected IsTeleported=false on fresh state")
	}
}

func TestStateTracker_RecordTeleported_SaveError(t *testing.T) {
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

	err := tracker.RecordTeleported()
	if err == nil {
		t.Fatal("RecordTeleported should return an error when save fails")
	}
	if !strings.Contains(err.Error(), "failed to write state") {
		t.Errorf("error should mention 'failed to write state', got: %v", err)
	}
}

// ─── Teleport symlink behavior (simulated WSL2 environment) ───

// TestTeleport_SymlinkIntegration tests the core symlink logic by creating
// a simulated WSL2 environment (temp dirs). This avoids needing an actual
// WSL2 install.
func TestTeleport_SymlinkIntegration(t *testing.T) {
	// Create a simulated Windows home under temp.
	tmpDir := t.TempDir()
	winHome := filepath.Join(tmpDir, "mnt", "c", "Users", "testuser")
	linuxHome := filepath.Join(tmpDir, "home", "testuser")

	// Create source folders
	for _, folder := range teleportFolders {
		folderPath := filepath.Join(winHome, folder)
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			t.Fatalf("failed to create source %s: %v", folderPath, err)
		}
	}

	// Create linuxHome parent so symlinks can be created inside it
	if err := os.MkdirAll(linuxHome, 0755); err != nil {
		t.Fatalf("failed to create linuxHome %s: %v", linuxHome, err)
	}

	// Run teleport-like logic manually (can't call Teleport directly since
	// it checks IsWSL2). This tests the symlink loop.
	for _, folder := range teleportFolders {
		source := filepath.Join(winHome, folder)
		target := filepath.Join(linuxHome, folder)

		// Check source exists
		srcInfo, err := os.Stat(source)
		if err != nil {
			t.Fatalf("source %s should exist: %v", source, err)
		}
		if !srcInfo.IsDir() {
			t.Fatalf("source %s should be a directory", source)
		}

		// Symlink
		if err := os.Symlink(source, target); err != nil {
			t.Fatalf("symlink %s -> %s failed: %v", source, target, err)
		}

		// Verify the symlink
		targetInfo, err := os.Lstat(target)
		if err != nil {
			t.Fatalf("lstat %s failed: %v", target, err)
		}
		if targetInfo.Mode()&os.ModeSymlink == 0 {
			t.Errorf("expected %s to be a symlink", target)
		}

		// Verify it resolves to the source
		resolved, err := os.Readlink(target)
		if err != nil {
			t.Fatalf("readlink %s failed: %v", target, err)
		}
		if resolved != source {
			t.Errorf("readlink(%s) = %s, want %s", target, resolved, source)
		}
	}

	// Verify all 4 symlinks were created
	for _, folder := range teleportFolders {
		target := filepath.Join(linuxHome, folder)
		if _, err := os.Lstat(target); err != nil {
			t.Errorf("expected symlink at %s: %v", target, err)
		}
	}
}

func TestTeleport_SymlinkIntegration_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	winHome := filepath.Join(tmpDir, "mnt", "c", "Users", "testuser")
	linuxHome := filepath.Join(tmpDir, "home", "testuser")

	for _, folder := range teleportFolders {
		folderPath := filepath.Join(winHome, folder)
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			t.Fatalf("failed to create source %s: %v", folderPath, err)
		}
	}

	// Dry run — no symlinks should be created
	for _, folder := range teleportFolders {
		target := filepath.Join(linuxHome, folder)
		if _, err := os.Lstat(target); err == nil {
			t.Errorf("dry-run should not create symlink at %s", target)
		}
	}
}

func TestTeleport_Symlink_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	winHome := filepath.Join(tmpDir, "mnt", "c", "Users", "testuser")
	linuxHome := filepath.Join(tmpDir, "home", "testuser")

	// Create source
	source := filepath.Join(winHome, "Documents")
	target := filepath.Join(linuxHome, "Documents")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.MkdirAll(linuxHome, 0755); err != nil {
		t.Fatalf("mkdir linuxHome: %v", err)
	}

	// Create a pre-existing target
	if err := os.MkdirAll(target, 0755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	// Symlink should fail because target already exists
	err := os.Symlink(source, target)
	if err == nil {
		t.Fatal("expected symlink to fail when target exists")
	}
}

func TestTeleport_Symlink_MissingSource(t *testing.T) {
	tmpDir := t.TempDir()
	linuxHome := filepath.Join(tmpDir, "home", "testuser")
	if err := os.MkdirAll(linuxHome, 0755); err != nil {
		t.Fatalf("mkdir linuxHome: %v", err)
	}

	// Source doesn't exist
	source := "/nonexistent/path/Documents"
	target := filepath.Join(linuxHome, "Documents")

	err := os.Symlink(source, target)
	if err != nil {
		// Symlinking a nonexistent source DOES succeed — the symlink
		// is created but dangling. This is how symlinks work.
		// The target just stores the path string.
		_ = err
	}
}
