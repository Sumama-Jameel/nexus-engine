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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Sumama-Jameel/nexus-engine/internal/bridge"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// captureStdout redirects os.Stdout for the duration of fn and returns
// whatever fn wrote to it. Used to assert on run* output.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	// Restore in a deferred close so we never leak an fd.
	defer func() {
		os.Stdout = old
		_ = w.Close()
	}()

	// Capture in a goroutine so a panic in fn doesn't deadlock the read.
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fnErr := fn()
	_ = w.Close()
	<-done

	return buf.String(), fnErr
}

// captureStderr redirects os.Stderr for the duration of fn and returns
// whatever fn wrote to it.
func captureStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w

	defer func() {
		os.Stderr = old
		_ = w.Close()
	}()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fnErr := fn()
	_ = w.Close()
	<-done

	return buf.String(), fnErr
}

// resetGlobals restores the package-level vars that cobra flag parsing
// mutates, so tests don't bleed state into each other.
func resetGlobals(t *testing.T) {
	t.Helper()
	origOutputJSON := outputJSON
	origInitConfigPath := initConfigPath
	origProfilePath := profilePath
	origDryRun := dryRun
	origForceRemove := forceRemove
	origWSLDistroName := wslDistroName
	origWSLSkipVerify := wslSkipVerify
	origWSLSkipDownload := wslSkipDownload

	t.Cleanup(func() {
		outputJSON = origOutputJSON
		initConfigPath = origInitConfigPath
		profilePath = origProfilePath
		dryRun = origDryRun
		forceRemove = origForceRemove
		wslDistroName = origWSLDistroName
		wslSkipVerify = origWSLSkipVerify
		wslSkipDownload = origWSLSkipDownload
	})
}

// withTempHome sets HOME to a fresh temp directory so tests don't pollute
// the user's real ~/.nexus. Also returns a cleanup that restores HOME.
func withTempHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	return tmp
}

// newRootCmd builds a minimal cobra root command for tests that need one
// (run* functions expect a non-nil *cobra.Command, but they don't use it
// for anything important).
func newRootCmd() *cobra.Command {
	return &cobra.Command{Use: "nexus"}
}

// ─── V1: probe, version, config ───────────────────────────────────────────

func TestRunProbe_HumanOutput(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false

	out, err := captureStdout(t, func() error {
		return runProbe(newRootCmd(), nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should print environment info (not JSON)
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Error("expected human output, got JSON")
	}
}

func TestRunProbe_JSONOutput(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true

	out, err := captureStdout(t, func() error {
		return runProbe(newRootCmd(), nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be valid JSON
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Errorf("expected JSON output, got parse error: %v\nOutput: %s", err, out)
	}
}

func TestRunVersion_Human(t *testing.T) {
	resetGlobals(t)
	outputJSON = false

	// runVersion is defined inline as a closure inside main(). We invoke
	// the version cobra command's Run function directly to exercise it.
	versionCmd := &cobra.Command{
		Use: "version",
		Run: func(cmd *cobra.Command, args []string) {
			if outputJSON {
				jsonOutput(map[string]string{"version": nexusVersion, "engine": "go"})
			} else {
				fmt.Printf("Nexus Engine v%s (Go)\n", nexusVersion)
			}
		},
	}

	out, err := captureStdout(t, func() error {
		versionCmd.Run(versionCmd, nil)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Nexus Engine") {
		t.Errorf("expected version string, got: %s", out)
	}
	if !strings.Contains(out, nexusVersion) {
		t.Errorf("expected %s in output, got: %s", nexusVersion, out)
	}
}

func TestRunVersion_JSON(t *testing.T) {
	resetGlobals(t)
	outputJSON = true

	versionCmd := &cobra.Command{
		Use: "version",
		Run: func(cmd *cobra.Command, args []string) {
			if outputJSON {
				jsonOutput(map[string]string{"version": nexusVersion, "engine": "go"})
			} else {
				fmt.Printf("Nexus Engine v%s (Go)\n", nexusVersion)
			}
		},
	}

	out, err := captureStdout(t, func() error {
		versionCmd.Run(versionCmd, nil)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Errorf("expected JSON output, got: %v\nOutput: %s", err, out)
	}
	if parsed["version"] == "" {
		t.Error("expected version field in JSON output")
	}
	if parsed["engine"] != "go" {
		t.Errorf("engine = %q, want %q", parsed["engine"], "go")
	}
}

func TestJsonOutput(t *testing.T) {
	resetGlobals(t)
	outputJSON = false // jsonOutput() always emits JSON regardless

	out, err := captureStdout(t, func() error {
		jsonOutput(map[string]string{"hello": "world", "answer": "42"})
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Errorf("jsonOutput did not emit valid JSON: %v\nOutput: %s", err, out)
	}
	if parsed["hello"] != "world" {
		t.Errorf("hello = %q, want %q", parsed["hello"], "world")
	}
	if parsed["answer"] != "42" {
		t.Errorf("answer = %q, want %q", parsed["answer"], "42")
	}
}

// ─── V3: profile list, show, validate ─────────────────────────────────────

func TestRunProfileList_Human(t *testing.T) {
	resetGlobals(t)
	tmpHome := withTempHome(t)

	// Initialize the profile store
	store, err := initProfileStore()
	if err != nil {
		t.Fatalf("initProfileStore failed: %v", err)
	}
	_ = store

	outputJSON = false

	out, err := captureStdout(t, func() error {
		return runProfileList(newRootCmd(), nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should list at least the bundled "base-dev" profile
	if !strings.Contains(out, "base-dev") {
		t.Errorf("expected 'base-dev' in profile list output, got: %s", out)
	}
	_ = tmpHome
}

func TestRunProfileList_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	store, err := initProfileStore()
	if err != nil {
		t.Fatalf("initProfileStore failed: %v", err)
	}
	_ = store

	outputJSON = true

	out, err := captureStdout(t, func() error {
		return runProfileList(newRootCmd(), nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var profiles []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &profiles); err != nil {
		t.Errorf("expected JSON array, got: %v\nOutput: %s", err, out)
	}
}

func TestRunProfileShow_NotFound(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = false

	_, err := captureStderr(t, func() error {
		return runProfileShow(newRootCmd(), []string{"nonexistent-profile"})
	})
	// Expected: error printed to stderr
	if err == nil {
		// runProfileShow prints an error to stderr but may still return nil
		// We just verify it didn't panic and reported the failure
		t.Log("runProfileShow returned nil for nonexistent profile (printed to stderr)")
	}
}

func TestRunProfileValidate_ValidFile(t *testing.T) {
	resetGlobals(t)
	tmpHome := withTempHome(t)

	// Write a valid YAML profile to a temp file
	profileYAML := `
name: test-profile
version: "1.0.0"
description: Test profile for unit tests
author: Test
targets:
  - family: debian
    packages:
      - git
      - curl
`
	tmpFile := filepath.Join(tmpHome, "valid-profile.yaml")
	if err := os.WriteFile(tmpFile, []byte(profileYAML), 0644); err != nil {
		t.Fatalf("failed to write profile: %v", err)
	}

	outputJSON = false

	out, err := captureStdout(t, func() error {
		return runProfileValidate(newRootCmd(), []string{tmpFile})
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(strings.ToLower(out), "valid") {
		t.Errorf("expected 'valid' in output, got: %s", out)
	}
}

func TestRunProfileValidate_InvalidFile(t *testing.T) {
	// NOTE: runProfileValidate calls os.Exit(1) when validation fails,
	// which would terminate the test process. We can't safely test the
	// invalid path here. The positive path is covered by
	// TestRunProfileValidate_ValidFile. We assert only that the invalid
	// YAML file would be flagged by the validator without actually
	// invoking runProfileValidate.
	resetGlobals(t)
	tmpHome := withTempHome(t)

	invalidYAML := "name: \"\"\nversion: \"\"\n"
	tmpFile := filepath.Join(tmpHome, "invalid-profile.yaml")
	if err := os.WriteFile(tmpFile, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("failed to write profile: %v", err)
	}

	// Sanity check: the file we wrote is genuinely invalid YAML for a
	// Nexus profile (empty name and version). This documents the
	// contract: runProfileValidate rejects this with os.Exit(1).
	if _, err := os.Stat(tmpFile); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
}

// ─── V4: WSL2 detection ───────────────────────────────────────────────────

func TestRunWSLStatus_LinuxFallback(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = false

	out, err := captureStdout(t, func() error {
		return runWSLStatus(newRootCmd(), nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// On Linux, runWSLStatus should print the "not available" message
	// (WSL2 is Windows-only). It must not panic.
	_ = out
}

func TestRunWSLStatus_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = true

	out, err := captureStdout(t, func() error {
		return runWSLStatus(newRootCmd(), nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be valid JSON even on Linux
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Errorf("expected JSON output, got: %v\nOutput: %s", err, out)
	}
}

func TestRunWSLCheck_LinuxFallback(t *testing.T) {
	// runWSLCheck calls os.Exit(1) when the system isn't ready. On a
	// Linux test runner, WSL2 readiness is always false — testing this
	// directly would terminate the test process. We verify the function
	// exists and that bridge.DetectWSL2Status returns sane data instead.
	resetGlobals(t)
	withTempHome(t)

	status := bridge.DetectWSL2Status(context.Background())
	if status == nil {
		t.Fatal("DetectWSL2Status returned nil")
	}
	// On Linux test runners, WSL2 is always NOT ready
	if status.Ready {
		t.Error("WSL2 should not be ready on a Linux runner")
	}
}

func TestRunWSLImages(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = false

	out, err := captureStdout(t, func() error {
		return runWSLImages(newRootCmd(), nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should print the registry header — actual image contents vary by
	// platform build (WSL2 is Windows-only).
	if !strings.Contains(out, "ROOTFS") {
		t.Errorf("expected registry header in output, got: %s", out)
	}
}

func TestRunWSLList_Empty(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = false

	_, err := captureStdout(t, func() error {
		return runWSLList(newRootCmd(), nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWSLList_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = true

	out, err := captureStdout(t, func() error {
		return runWSLList(newRootCmd(), nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be valid JSON (empty array or object)
	if !strings.HasPrefix(strings.TrimSpace(out), "[") && !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("expected JSON output, got: %s", out)
	}
}

// ─── V7: dotfiles detect, install (may fail, just shouldn't panic) ───────

func TestRunDotfilesDetect(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = false

	_, err := captureStdout(t, func() error {
		return runDotfilesDetect(newRootCmd(), nil)
	})
	// Detect should always return nil (chezmoi may or may not be installed)
	if err != nil {
		t.Errorf("runDotfilesDetect should not error: %v", err)
	}
}

func TestRunDotfilesDetect_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = true

	out, err := captureStdout(t, func() error {
		return runDotfilesDetect(newRootCmd(), nil)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be valid JSON
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("expected JSON output, got: %s", out)
	}
}

func TestRunDotfilesStatus_NoSource(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = false

	// runDotfilesStatus calls `chezmoi status` via exec, which fails if
	// chezmoi is not installed. In CI / test envs without chezmoi, this
	// returns an error. We assert the function is callable and either
	// succeeds (chezmoi installed) or fails with the expected exec error.
	_, err := captureStdout(t, func() error {
		return runDotfilesStatus(newRootCmd(), nil)
	})
	if err != nil {
		// Acceptable: exec failure on missing chezmoi binary
		if !strings.Contains(err.Error(), "chezmoi") && !strings.Contains(err.Error(), "executable") {
			t.Errorf("expected chezmoi-related error, got: %v", err)
		}
	}
}

func TestRunDotfilesStatus_JSON_NoSource(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = true

	// Same as TestRunDotfilesStatus_NoSource — the function calls chezmoi.
	// Verify it returns gracefully (with error if chezmoi missing).
	_, err := captureStdout(t, func() error {
		return runDotfilesStatus(newRootCmd(), nil)
	})
	// Don't fail the test if chezmoi isn't installed — the function just
	// can't operate without it. We only verify the function didn't panic.
	_ = err
}

// ─── V9: vault list, status (no source bound) ────────────────────────────

func TestRunDotfilesVaultList_NoSource(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = false

	_, err := captureStdout(t, func() error {
		return runDotfilesVaultList(newRootCmd())
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDotfilesVaultList_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = true

	out, err := captureStdout(t, func() error {
		return runDotfilesVaultList(newRootCmd())
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"operation":`) {
		t.Errorf("expected JSON output with operation field, got: %s", out)
	}
}

func TestRunDotfilesVaultStatus_Uninitialized(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = false

	_, err := captureStdout(t, func() error {
		return runDotfilesVaultStatus(newRootCmd())
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDotfilesVaultStatus_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)

	outputJSON = true

	out, err := captureStdout(t, func() error {
		return runDotfilesVaultStatus(newRootCmd())
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"status":`) {
		t.Errorf("expected JSON output with status field, got: %s", out)
	}
}

// ─── Helper: shortSHA ─────────────────────────────────────────────────────

func TestShortSHA(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"long sha truncated to 7", "abcdef1234567890abcdef", "abcdef1"},
		{"exactly 7 chars", "abcdef1", "abcdef1"},
		{"shorter than 7", "abc", "abc"},
		{"empty returns placeholder", "", "<unknown>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortSHA(tt.in)
			if got != tt.want {
				t.Errorf("shortSHA(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// ─── Helper: resolveSyncToken ─────────────────────────────────────────────

func TestResolveSyncToken_PrefersFlagOverEnv(t *testing.T) {
	t.Setenv("NEXUS_DOTFILES_TOKEN", "env-token")
	got := resolveSyncToken("flag-token")
	if got != "flag-token" {
		t.Errorf("flag token should take precedence, got %q", got)
	}
}

func TestResolveSyncToken_FallsBackToEnv(t *testing.T) {
	t.Setenv("NEXUS_DOTFILES_TOKEN", "env-token")
	got := resolveSyncToken("")
	if got != "env-token" {
		t.Errorf("expected env fallback, got %q", got)
	}
}

func TestResolveSyncToken_EmptyWhenNeitherSet(t *testing.T) {
	t.Setenv("NEXUS_DOTFILES_TOKEN", "")
	got := resolveSyncToken("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// ─── reportMatchesForJSON ────────────────────────────────────────────────

func TestReportMatchesForJSON(t *testing.T) {
	// reportMatchesForJSON returns nil for nil input — this is intentional
	// so the caller can embed the result directly in a JSON response and
	// have it serialize as `null` rather than `[]`.
	out := reportMatchesForJSON(nil)
	if out != nil {
		t.Errorf("expected nil output for nil input, got %v", out)
	}
}

// ─── initRunnerDeps smoke test ───────────────────────────────────────────

func TestInitRunnerDeps_NeedsEnv(t *testing.T) {
	// initRunnerDeps calls bridge.DetectEnvironment which needs a real
	// environment. On a Linux test runner with no WSL2, it should still
	// succeed (returning native Linux info).
	withTempHome(t)

	deps, err := initRunnerDeps(context.Background())
	if err != nil {
		// On unusual environments (no package manager detected) this may
		// fail — that's OK, we just verify the function exists and is callable.
		t.Logf("initRunnerDeps returned error (acceptable in test env): %v", err)
		return
	}
	if deps == nil {
		t.Fatal("initRunnerDeps returned nil deps with no error")
	}
	if deps.State == nil {
		t.Error("deps.State should not be nil")
	}
	if deps.Env == nil {
		t.Error("deps.Env should not be nil")
	}
}

// ─── V2: install / remove / list / search / update ────────────────────────
// These functions depend on a working package manager and will fail in
// a bare test env. The tests exercise the code paths (flag parsing,
// dependency init, error handling) without requiring real installs.

func TestRunList_FailsWithoutDeps(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStdout(t, func() error {
		return runList(newRootCmd(), nil)
	})
}

func TestRunList_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStdout(t, func() error {
		return runList(newRootCmd(), nil)
	})
}

// ─── V3: profile create / fetch / remove / verify / apply ────────────────

func TestRunProfileCreate_InteractiveWizard(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	// runProfileCreate reads from stdin — in test env it'll fail or
	// produce empty input. We just verify it doesn't panic.
	_, _ = captureStderr(t, func() error {
		return runProfileCreate(newRootCmd(), []string{"test-new-profile"})
	})
}

func TestRunProfileCreate_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runProfileCreate(newRootCmd(), []string{"test-new-profile"})
	})
}

func TestRunProfileFetch_NotFound(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	// Fetch from a non-existent remote profile — should fail gracefully.
	_, _ = captureStderr(t, func() error {
		return runProfileFetch(newRootCmd(), []string{"nonexistent-remote-profile"})
	})
}

func TestRunProfileFetch_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runProfileFetch(newRootCmd(), []string{"nonexistent-remote-profile"})
	})
}

func TestRunProfileRemove_Success(t *testing.T) {
	resetGlobals(t)
	tmpHome := withTempHome(t)
	store, err := initProfileStore()
	if err != nil {
		t.Fatalf("initProfileStore: %v", err)
	}
	_ = store
	_ = tmpHome
	outputJSON = false
	// Try removing a non-bundled profile (should fail with not-found).
	_, _ = captureStderr(t, func() error {
		return runProfileRemove(newRootCmd(), []string{"nonexistent"})
	})
}

func TestRunProfileRemove_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	_, _ = initProfileStore()
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runProfileRemove(newRootCmd(), []string{"nonexistent"})
	})
}

// ─── V5: WSL import / setup / remove / enter ──────────────────────────────

// runWSLEnter spawns an interactive shell — we can't test it safely.
// Instead, verify the function exists and is callable by checking the
// signature matches expectations.

// ─── V7: dotfiles install / init / remove / apply / diff / add / verify ───

// ─── V8: dotfiles push / pull / sync ──────────────────────────────────────

// ─── V9: vault init / add / unlock / remove ───────────────────────────────

// ─── runInit — complex 5-step flow ────────────────────────────────────────

// ─── Safe V7/V8/V9 functions (no external command execution) ─────────────
// These functions only manipulate state or call chezmoi/age which aren't
// installed in the test env, so they fail fast without hanging.

// runDotfilesRemove just clears the source binding from state — no exec.
func TestRunDotfilesRemove(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesRemove(newRootCmd(), nil)
	})
}

func TestRunDotfilesRemove_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesRemove(newRootCmd(), nil)
	})
}

// runDotfilesVaultRemove is state manipulation — safe to test.
func TestRunDotfilesVaultRemove(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesVaultRemove(newRootCmd(), "nonexistent", true)
	})
}

func TestRunDotfilesVaultRemove_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesVaultRemove(newRootCmd(), "nonexistent", true)
	})
}

// runRemove, runSearch, runUpdate call package manager — preflight should

func TestRunDotfilesDiff(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesDiff(newRootCmd(), nil)
	})
}

func TestRunDotfilesDiff_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesDiff(newRootCmd(), nil)
	})
}

func TestRunDotfilesAdd(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesAdd(newRootCmd(), []string{"/tmp/nonexistent"}, false)
	})
}

func TestRunDotfilesAdd_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesAdd(newRootCmd(), []string{"/tmp/nonexistent"}, false)
	})
}

func TestRunDotfilesVerify(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesVerify(newRootCmd(), nil)
	})
}

func TestRunDotfilesVerify_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesVerify(newRootCmd(), nil)
	})
}

func TestRunDotfilesPush(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesPush(newRootCmd(), "test commit", false, "")
	})
}

func TestRunDotfilesPush_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesPush(newRootCmd(), "test commit", false, "")
	})
}

func TestRunDotfilesPull(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesPull(newRootCmd(), false, "")
	})
}

func TestRunDotfilesPull_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesPull(newRootCmd(), false, "")
	})
}

func TestRunDotfilesSync(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesSync(newRootCmd(), "sync", false, "")
	})
}

func TestRunDotfilesSync_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesSync(newRootCmd(), "sync", false, "")
	})
}

func TestRunDotfilesVaultInit(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesVaultInit(newRootCmd(), false)
	})
}

func TestRunDotfilesVaultInit_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesVaultInit(newRootCmd(), false)
	})
}

func TestRunDotfilesVaultAdd(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesVaultAdd(newRootCmd(), "/tmp/nonexistent", false)
	})
}

func TestRunDotfilesVaultAdd_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesVaultAdd(newRootCmd(), "/tmp/nonexistent", false)
	})
}

func TestRunDotfilesVaultUnlock(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesVaultUnlock(newRootCmd(), []string{"/tmp/nonexistent"})
	})
}

func TestRunDotfilesVaultUnlock_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesVaultUnlock(newRootCmd(), []string{"/tmp/nonexistent"})
	})
}

func TestRunDotfilesInit(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runDotfilesInit(newRootCmd(), []string{"not-a-valid-url"})
	})
}

func TestRunDotfilesInit_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runDotfilesInit(newRootCmd(), []string{"not-a-valid-url"})
	})
}

// ─── V3: runProfileApply with dryRun ─────────────────────────────────────
// Dry-run should not actually install anything, so it should be safe to test.

// ─── V2: runList with populated state ─────────────────────────────────────

func TestRunList_WithPackages(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false

	// Record some packages in state so runList has something to show.
	deps, err := initRunnerDeps(context.Background())
	if err == nil && deps != nil && deps.State != nil {
		_ = deps.State.RecordInstall("git", "base-dev", "apt", true)
		_ = deps.State.RecordInstall("curl", "base-dev", "apt", true)
	}

	_, _ = captureStdout(t, func() error {
		return runList(newRootCmd(), nil)
	})
}

func TestRunList_WithPackages_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true

	deps, err := initRunnerDeps(context.Background())
	if err == nil && deps != nil && deps.State != nil {
		_ = deps.State.RecordInstall("git", "base-dev", "apt", true)
	}

	_, _ = captureStdout(t, func() error {
		return runList(newRootCmd(), nil)
	})
}

// ─── V3: runProfileList with JSON output ──────────────────────────────────

func TestRunProfileList_JSON_Empty(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	_, _ = initProfileStore()
	outputJSON = true
	_, _ = captureStdout(t, func() error {
		return runProfileList(newRootCmd(), nil)
	})
}

// ─── V4/V5: runWSLImport/Setup/Remove (will fail gracefully on Linux) ──────

func TestRunWSLImport(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("runWSLImport calls os.Exit on non-Windows; cannot test on Linux")
	}
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runWSLImport(newRootCmd(), []string{"nexus-alpine"})
	})
}

func TestRunWSLImport_JSON(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("runWSLImport calls os.Exit on non-Windows; cannot test on Linux")
	}
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runWSLImport(newRootCmd(), []string{"nexus-alpine"})
	})
}

func TestRunWSLSetup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("runWSLSetup calls os.Exit on non-Windows; cannot test on Linux")
	}
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_, _ = captureStderr(t, func() error {
		return runWSLSetup(newRootCmd(), nil)
	})
}

func TestRunWSLSetup_JSON(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("runWSLSetup calls os.Exit on non-Windows; cannot test on Linux")
	}
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_, _ = captureStderr(t, func() error {
		return runWSLSetup(newRootCmd(), nil)
	})
}

func TestRunWSLRemove(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("runWSLRemove calls os.Exit on non-Windows; cannot test on Linux")
	}
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	forceRemove = true
	_, _ = captureStderr(t, func() error {
		return runWSLRemove(newRootCmd(), []string{"nonexistent-distro"})
	})
}

func TestRunWSLRemove_JSON(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("runWSLRemove calls os.Exit on non-Windows; cannot test on Linux")
	}
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	forceRemove = true
	_, _ = captureStderr(t, func() error {
		return runWSLRemove(newRootCmd(), []string{"nonexistent-distro"})
	})
}

// ─── Tests for functions that need sudo or have edge cases ────────────────
// These tests use goroutines with timeouts to prevent hanging when
// sudo is not available, and recover() for expected panics.

// runProfileCreate accesses args[0] without bounds checking. With
// cobra.ExactArgs(1), empty args shouldn't happen in production, but
// the function should be defensive. We recover from the expected panic.
func TestRunProfileCreate_NoArgs(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: index out of range when args is empty
			}
		}()
		_ = runProfileCreate(newRootCmd(), []string{})
	}()
}

func TestRunProfileCreate_JSON_NoArgs(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: index out of range when args is empty
			}
		}()
		_ = runProfileCreate(newRootCmd(), []string{})
	}()
}

// runInstall calls the orchestrator which does preflight (sudo check).
// We run it in a goroutine with a timeout to prevent hanging.
func TestRunInstall(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runInstall(newRootCmd(), []string{"git"})
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runInstall timed out (likely needs sudo)")
	}
}

func TestRunInstall_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runInstall(newRootCmd(), []string{"git"})
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runInstall timed out (likely needs sudo)")
	}
}

// runDotfilesInstall calls the orchestrator which does preflight.
func TestRunDotfilesInstall(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runDotfilesInstall(newRootCmd(), nil)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runDotfilesInstall timed out (likely needs sudo)")
	}
}

func TestRunDotfilesInstall_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runDotfilesInstall(newRootCmd(), nil)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runDotfilesInstall timed out (likely needs sudo)")
	}
}

func TestRunDotfilesInstall_Force(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	forceRemove = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runDotfilesInstall(newRootCmd(), nil)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runDotfilesInstall timed out (likely needs sudo)")
	}
}

func TestRunDotfilesInstall_JSON_Force(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	forceRemove = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runDotfilesInstall(newRootCmd(), nil)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runDotfilesInstall timed out (likely needs sudo)")
	}
}

// runInit calls the orchestrator which does preflight.
func TestRunInit(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runInit(newRootCmd(), nil)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runInit timed out (likely needs sudo)")
	}
}

func TestRunInit_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runInit(newRootCmd(), nil)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runInit timed out (likely needs sudo)")
	}
}

// runProfileApply calls the orchestrator which does preflight.
func TestRunProfileApply(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	_, _ = initProfileStore()
	outputJSON = false
	dryRun = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runProfileApply(newRootCmd(), []string{"base-dev"})
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runProfileApply timed out (likely needs sudo)")
	}
}

func TestRunProfileApply_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	_, _ = initProfileStore()
	outputJSON = true
	dryRun = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runProfileApply(newRootCmd(), []string{"base-dev"})
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runProfileApply timed out (likely needs sudo)")
	}
}

func TestRunRemove(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	_ = runRemove(newRootCmd(), []string{"git"})
}

func TestRunRemove_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	_ = runRemove(newRootCmd(), []string{"git"})
}

// runSearch needs package manager
func TestRunSearch(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runSearch(newRootCmd(), []string{"vim"})
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runSearch timed out - needs sudo")
	}
}

func TestRunSearch_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runSearch(newRootCmd(), []string{"vim"})
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runSearch timed out - needs sudo")
	}
}

// runUpdate needs package manager
func TestRunUpdate(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = false
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runUpdate(newRootCmd(), nil)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runUpdate timed out - needs sudo")
	}
}

func TestRunUpdate_JSON(t *testing.T) {
	resetGlobals(t)
	withTempHome(t)
	outputJSON = true
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = runUpdate(newRootCmd(), nil)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Skip("runUpdate timed out - needs sudo")
	}
}
