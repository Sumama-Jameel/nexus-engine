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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// generateEnvExports — env var sanitization and formatting
// ---------------------------------------------------------------------------

func TestGenerateEnvExports_BasicVars(t *testing.T) {
	t.Parallel()

	envVars := map[string]string{
		"EDITOR":  "vim",
		"LANG":    "en_US.UTF-8",
		"PROJECT": "nexus",
	}

	output := generateEnvExports(envVars)

	if !strings.Contains(output, "export EDITOR=\"vim\"") {
		t.Errorf("missing export for EDITOR, got: %s", output)
	}
	if !strings.Contains(output, "export LANG=\"en_US.UTF-8\"") {
		t.Errorf("missing export for LANG, got: %s", output)
	}
	if !strings.Contains(output, "export PROJECT=\"nexus\"") {
		t.Errorf("missing export for PROJECT, got: %s", output)
	}
}

func TestGenerateEnvExports_SortedOrder(t *testing.T) {
	t.Parallel()

	envVars := map[string]string{
		"ZEBRA":  "last",
		"ALPHA":  "first",
		"MIDDLE": "mid",
	}

	output := generateEnvExports(envVars)

	alphaIdx := strings.Index(output, "ALPHA")
	middleIdx := strings.Index(output, "MIDDLE")
	zebraIdx := strings.Index(output, "ZEBRA")

	if alphaIdx >= middleIdx {
		t.Error("ALPHA should appear before MIDDLE")
	}
	if middleIdx >= zebraIdx {
		t.Error("MIDDLE should appear before ZEBRA")
	}
}

func TestGenerateEnvExports_Empty(t *testing.T) {
	t.Parallel()

	output := generateEnvExports(nil)
	if output != "" {
		t.Errorf("empty env vars should produce empty output, got: %q", output)
	}

	output = generateEnvExports(map[string]string{})
	if output != "" {
		t.Errorf("empty env vars map should produce empty output, got: %q", output)
	}
}

func TestGenerateEnvExports_SpecialCharactersSanitized(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"semicolon", "EVIL", "value;rm -rf /"},
		{"pipe", "EVIL", "value|sh"},
		{"dollar", "EVIL", "$HOME"},
		{"backtick", "EVIL", "`whoami`"},
		{"newline", "EVIL", "foo\nbar"},
		{"redirect", "EVIL", ">file"},
		{"ampersand", "EVIL", "&&evil"},
		{"single_quote", "EVIL", "it's"},
		{"double_quote", "EVIL", `val"ue`},
		{"backslash", "EVIL", `val\ue`},
		{"command_substitution", "EVIL", "$(cat /etc/passwd)"},
		{"exclamation", "EVIL", "!event"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			output := generateEnvExports(map[string]string{tt.key: tt.value})
			if !strings.Contains(output, "WARNING") {
				t.Errorf("value %q should trigger WARNING, got: %s", tt.value, output)
			}
		})
	}
}

func TestGenerateEnvExports_SafeSpecialValues(t *testing.T) {
	t.Parallel()

	safe := map[string]string{
		"PATH_EXTRA": "/usr/local/bin:/usr/bin",
		"VERSION":    "1.2.3-beta",
		"NUMBER":     "42",
		"SPACES":     "hello world",
		"TILDE":      "~/path",
		"HASH":       "#comment",
	}

	output := generateEnvExports(safe)
	for k := range safe {
		if !strings.Contains(output, "export "+k+"=") {
			t.Errorf("safe env var %q should be exported, got: %s", k, output)
		}
	}
}

// ---------------------------------------------------------------------------
// generateZshConfig / generateBashConfig
// ---------------------------------------------------------------------------

func TestGenerateZshConfig(t *testing.T) {
	t.Parallel()

	envVars := map[string]string{
		"NEXUS_PROFILE": "base-dev",
	}

	output := generateZshConfig(envVars)

	if !strings.Contains(output, "NEXUS_START") {
		t.Error("zsh config missing NEXUS_START marker")
	}
	if !strings.Contains(output, "NEXUS_END") {
		t.Error("zsh config missing NEXUS_END marker")
	}
	if !strings.Contains(output, "NEXUS_PROFILE") {
		t.Error("zsh config missing NEXUS_PROFILE env var")
	}
	if !strings.Contains(output, "NEXUS_HOME") {
		t.Error("zsh config missing NEXUS_HOME")
	}
	if !strings.Contains(output, "HISTSIZE") {
		t.Error("zsh config missing HISTSIZE")
	}
}

func TestGenerateBashConfig(t *testing.T) {
	t.Parallel()

	envVars := map[string]string{
		"NEXUS_PROFILE": "base-dev",
	}

	output := generateBashConfig(envVars)

	if !strings.Contains(output, "NEXUS_START") {
		t.Error("bash config missing NEXUS_START marker")
	}
	if !strings.Contains(output, "NEXUS_END") {
		t.Error("bash config missing NEXUS_END marker")
	}
	if !strings.Contains(output, "NEXUS_PROFILE") {
		t.Error("bash config missing NEXUS_PROFILE env var")
	}
	if !strings.Contains(output, "NEXUS_HOME") {
		t.Error("bash config missing NEXUS_HOME")
	}
}

// ---------------------------------------------------------------------------
// injectShellConfig — marker-based injection and idempotent replacement
// ---------------------------------------------------------------------------

func TestInjectShellConfig_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bashrc")

	config := generateBashConfig(map[string]string{"FOO": "bar"})

	err := injectShellConfig(configPath, config)
	if err != nil {
		t.Fatalf("injectShellConfig on new file failed: %v", err)
	}

	data, err := os.ReadFile(configPath) //nolint:gosec
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "NEXUS_START") {
		t.Error("new config should contain NEXUS_START marker")
	}
	if !strings.Contains(content, "export FOO") {
		t.Error("new config should contain env var export")
	}
}

func TestInjectShellConfig_AppendToExisting(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bashrc")

	existing := "# My existing bash config\nalias ll='ls -la'\n"
	_ = os.WriteFile(configPath, []byte(existing), 0644) //nolint:gosec

	config := generateBashConfig(map[string]string{"BAR": "baz"})

	err := injectShellConfig(configPath, config)
	if err != nil {
		t.Fatalf("injectShellConfig on existing file failed: %v", err)
	}

	data, _ := os.ReadFile(configPath) //nolint:gosec
	content := string(data)

	if !strings.Contains(content, "alias ll='ls -la'") {
		t.Error("existing config should be preserved")
	}
	if !strings.Contains(content, "NEXUS_START") {
		t.Error("new config should contain NEXUS_START marker")
	}
	if !strings.Contains(content, "export BAR") {
		t.Error("new config should contain env var export")
	}
}

func TestInjectShellConfig_IdempotentReplacement(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bashrc")

	// First injection
	config1 := generateBashConfig(map[string]string{"VERSION": "1"})
	err := injectShellConfig(configPath, config1)
	if err != nil {
		t.Fatalf("first injectShellConfig failed: %v", err)
	}

	// Second injection with updated env vars — should REPLACE, not duplicate
	config2 := generateBashConfig(map[string]string{"VERSION": "2", "EXTRA": "val"})
	err = injectShellConfig(configPath, config2)
	if err != nil {
		t.Fatalf("second injectShellConfig failed: %v", err)
	}

	data2, _ := os.ReadFile(configPath)

	// Should contain the updated value
	if !strings.Contains(string(data2), "VERSION") {
		t.Error("should contain VERSION env var")
	}
	if !strings.Contains(string(data2), "EXTRA") {
		t.Error("should contain EXTRA env var")
	}

	// Should NOT contain two NEXUS_START markers (duplicated blocks)
	startCount := strings.Count(string(data2), "NEXUS_START")
	if startCount != 1 {
		t.Errorf("expected 1 NEXUS_START marker, got %d (block was duplicated)", startCount)
	}
}

// ---------------------------------------------------------------------------
// detectShell
// ---------------------------------------------------------------------------

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name     string
		shellEnv string
		expected string
	}{
		{"zsh", "/bin/zsh", "zsh"},
		{"zsh_with_path", "/usr/bin/zsh", "zsh"},
		{"bash", "/bin/bash", "bash"},
		{"bash_with_path", "/usr/local/bin/bash", "bash"},
		{"empty", "", "bash"},
		{"fish", "/usr/bin/fish", "bash"},
		{"sh", "/bin/sh", "bash"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			origShell := os.Getenv("SHELL")
			_ = os.Setenv("SHELL", tt.shellEnv)                  //nolint:errcheck // test setup
			defer func() { _ = os.Setenv("SHELL", origShell) }() //nolint:errcheck // test cleanup

			got := detectShell()
			if got != tt.expected {
				t.Errorf("detectShell() with SHELL=%q = %q, want %q", tt.shellEnv, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Configure — integration test (creates directory, injects config)
// ---------------------------------------------------------------------------

func TestConfigure_CreatesNexusDir(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)                      //nolint:errcheck // test setup
	defer func() { _ = os.Setenv("HOME", origHome) }() //nolint:errcheck // test cleanup

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")                  //nolint:errcheck // test setup
	defer func() { _ = os.Setenv("SHELL", origShell) }() //nolint:errcheck // test cleanup

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"FOO": "bar"})

	if !result.NexusDirCreated {
		t.Error("NexusDirCreated should be true")
	}
	expectedDir := filepath.Join(tmpDir, ".nexus")
	if result.NexusDir != expectedDir {
		t.Errorf("NexusDir = %q, want %q", result.NexusDir, expectedDir)
	}

	info, err := os.Stat(expectedDir)
	if err != nil {
		t.Fatalf("nexus directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("nexus path should be a directory")
	}
}

func TestConfigure_InjectsEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)                      //nolint:errcheck // test setup
	defer func() { _ = os.Setenv("HOME", origHome) }() //nolint:errcheck // test cleanup

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")                  //nolint:errcheck // test setup
	defer func() { _ = os.Setenv("SHELL", origShell) }() //nolint:errcheck // test cleanup

	ctx, cancel := contextWithTimeout()
	defer cancel()

	envVars := map[string]string{
		"MY_VAR": "my_value",
	}

	result := Configure(ctx, envVars)

	if result.EnvVarsApplied != 1 {
		t.Errorf("EnvVarsApplied = %d, want 1", result.EnvVarsApplied)
	}
}

func TestConfigure_SanitizesBadEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)                      //nolint:errcheck // test setup
	defer func() { _ = os.Setenv("HOME", origHome) }() //nolint:errcheck // test cleanup

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")                  //nolint:errcheck // test setup
	defer func() { _ = os.Setenv("SHELL", origShell) }() //nolint:errcheck // test cleanup

	ctx, cancel := contextWithTimeout()
	defer cancel()

	envVars := map[string]string{
		"GOOD_VAR": "safe_value",
		"BAD_VAR":  "value;rm -rf /",
	}

	result := Configure(ctx, envVars)

	if result.ShellConfigWritten {
		data, err := os.ReadFile(result.ShellConfigPath)
		if err == nil {
			content := string(data)
			if !strings.Contains(content, "WARNING") {
				t.Error("shell config should contain WARNING for bad env var")
			}
			if !strings.Contains(content, "export GOOD_VAR") {
				t.Error("shell config should contain GOOD_VAR export")
			}
		}
	}
}

// ---------------------------------------------------------------------------
// ConfigureResult — JSON serialization
// ---------------------------------------------------------------------------

func TestConfigureResult_JSONFields(t *testing.T) {
	t.Parallel()

	result := &ConfigureResult{
		ChezmoiInstalled:   true,
		ChezmoiInitialized: true,
		ShellConfigWritten: true,
		ShellConfigPath:    "/home/user/.zshrc",
		ShellType:          "zsh",
		NexusDirCreated:    true,
		NexusDir:           "/home/user/.nexus",
		EnvVarsApplied:     3,
		Messages:           []string{"Created /home/user/.nexus"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var raw map[string]interface{}
	_ = json.Unmarshal(data, &raw)

	expectedFields := []string{
		"chezmoi_installed", "chezmoi_initialized", "shell_config_written",
		"shell_config_path", "shell_type", "nexus_dir_created",
		"nexus_dir", "env_vars_applied", "messages",
	}

	for _, f := range expectedFields {
		if _, ok := raw[f]; !ok {
			t.Errorf("ConfigureResult JSON missing field %q", f)
		}
	}
}

// Helper
func contextWithTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// ---------------------------------------------------------------------------
// Configure — additional edge case tests
// ---------------------------------------------------------------------------

func TestConfigure_ZshShell(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)                      //nolint:errcheck // test setup
	defer func() { _ = os.Setenv("HOME", origHome) }() //nolint:errcheck // test cleanup

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/zsh")                   //nolint:errcheck // test setup
	defer func() { _ = os.Setenv("SHELL", origShell) }() //nolint:errcheck // test cleanup

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"TEST_VAR": "zsh_value"})

	if result.ShellType != "zsh" {
		t.Errorf("ShellType = %q, want %q", result.ShellType, "zsh")
	}
	if result.ShellConfigPath != filepath.Join(tmpDir, ".zshrc") {
		t.Errorf("ShellConfigPath = %q, want %q", result.ShellConfigPath, filepath.Join(tmpDir, ".zshrc"))
	}
	if result.ShellConfigWritten {
		data, err := os.ReadFile(result.ShellConfigPath)
		if err != nil {
			t.Fatalf("failed to read .zshrc: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, "NEXUS_START") {
			t.Error("zsh config should contain NEXUS_START marker")
		}
		if !strings.Contains(content, "TEST_VAR") {
			t.Error("zsh config should contain TEST_VAR export")
		}
	}
}

func TestConfigure_DefaultShell(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/fish") // not bash or zsh
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{})

	// Should default to bash
	if result.ShellType != "bash" {
		t.Errorf("ShellType = %q, want %q (default for unknown shell)", result.ShellType, "bash")
	}
}

func TestConfigure_EmptyEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, nil)

	if result.EnvVarsApplied != 0 {
		t.Errorf("EnvVarsApplied = %d, want 0 for nil env vars", result.EnvVarsApplied)
	}
}

func TestConfigure_NexusDirAlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	// Pre-create the .nexus directory
	nexusDir := filepath.Join(tmpDir, ".nexus")
	_ = os.MkdirAll(nexusDir, 0755) //nolint:gosec

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"X": "y"})

	// NexusDirCreated should still be true (MkdirAll succeeds even if exists)
	if !result.NexusDirCreated {
		t.Error("NexusDirCreated should be true even when directory already exists")
	}
}

func TestConfigure_ShellConfigWritten(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"EDITOR": "vim"})

	if !result.ShellConfigWritten {
		t.Error("ShellConfigWritten should be true")
	}
	if result.EnvVarsApplied != 1 {
		t.Errorf("EnvVarsApplied = %d, want 1", result.EnvVarsApplied)
	}

	// Verify the shell config file was actually written
	data, err := os.ReadFile(result.ShellConfigPath)
	if err != nil {
		t.Fatalf("failed to read shell config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "export EDITOR=\"vim\"") {
		t.Error("shell config should contain EDITOR export")
	}
}

func TestConfigureResult_Messages(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{})

	// Should have at least a message about creating the nexus dir
	if len(result.Messages) == 0 {
		t.Error("ConfigureResult should have at least one message")
	}

	foundNexusDirMsg := false
	for _, msg := range result.Messages {
		if strings.Contains(msg, ".nexus") {
			foundNexusDirMsg = true
			break
		}
	}
	if !foundNexusDirMsg {
		t.Errorf("expected a message about .nexus directory, got: %v", result.Messages)
	}
}

// ---------------------------------------------------------------------------
// Configure — Chezmoi branches
// ---------------------------------------------------------------------------

// createFakeChezmoiBin creates a fake "chezmoi" script in a temp bin directory
// and returns the bin directory path. The script's behavior is controlled by
// the initExit and initOutput parameters.
func createFakeChezmoiBin(t *testing.T, initExit int, initOutput string) string {
	t.Helper()
	binDir := t.TempDir()
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"init\" ]; then\n  echo '%s'\n  exit %d\nfi\nexit 0\n", initOutput, initExit)
	chezmoiPath := filepath.Join(binDir, "chezmoi")
	if err := os.WriteFile(chezmoiPath, []byte(script), 0755); err != nil { //nolint:gosec
		t.Fatalf("failed to create fake chezmoi: %v", err)
	}
	return binDir
}

func TestConfigure_ChezmoiNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	// Ensure chezmoi is NOT on PATH
	origPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", tmpDir)
	defer func() { _ = os.Setenv("PATH", origPath) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"A": "1"})

	if result.ChezmoiInstalled {
		t.Error("ChezmoiInstalled should be false when chezmoi is not on PATH")
	}
	if result.ChezmoiInitialized {
		t.Error("ChezmoiInitialized should be false when chezmoi is not installed")
	}

	// Check that the message about chezmoi not found is present
	foundChezmoiMsg := false
	for _, msg := range result.Messages {
		if strings.Contains(msg, "Chezmoi not found") {
			foundChezmoiMsg = true
			break
		}
	}
	if !foundChezmoiMsg {
		t.Errorf("expected 'Chezmoi not found' message, got: %v", result.Messages)
	}

	// Check install hint message
	foundInstallMsg := false
	for _, msg := range result.Messages {
		if strings.Contains(msg, "Install Chezmoi") {
			foundInstallMsg = true
			break
		}
	}
	if !foundInstallMsg {
		t.Errorf("expected 'Install Chezmoi' hint message, got: %v", result.Messages)
	}
}

func TestConfigure_ChezmoiFound_AlreadyInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	// Create a fake chezmoi binary on PATH
	binDir := createFakeChezmoiBin(t, 0, "chezmoi initialized")
	origPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", binDir+":"+origPath)
	defer func() { _ = os.Setenv("PATH", origPath) }()

	// Pre-create the chezmoi data directory to simulate already initialized
	chezmoiDir := filepath.Join(tmpDir, ".local", "share", "chezmoi")
	if err := os.MkdirAll(chezmoiDir, 0755); err != nil { //nolint:gosec
		t.Fatalf("failed to create chezmoi dir: %v", err)
	}

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"B": "2"})

	if !result.ChezmoiInstalled {
		t.Error("ChezmoiInstalled should be true when chezmoi is on PATH")
	}
	if !result.ChezmoiInitialized {
		t.Error("ChezmoiInitialized should be true when chezmoi dir already exists")
	}

	// Check message about already initialized
	foundMsg := false
	for _, msg := range result.Messages {
		if strings.Contains(msg, "Chezmoi already initialized") {
			foundMsg = true
			break
		}
	}
	if !foundMsg {
		t.Errorf("expected 'Chezmoi already initialized' message, got: %v", result.Messages)
	}
}

func TestConfigure_ChezmoiFound_InitSucceeds(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	// Create a fake chezmoi that succeeds on init with output
	binDir := createFakeChezmoiBin(t, 0, "chezmoi init output here")
	origPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", binDir+":"+origPath)
	defer func() { _ = os.Setenv("PATH", origPath) }()

	// Do NOT create chezmoi data dir — simulate first-time init

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"C": "3"})

	if !result.ChezmoiInstalled {
		t.Error("ChezmoiInstalled should be true")
	}
	if !result.ChezmoiInitialized {
		t.Error("ChezmoiInitialized should be true when init succeeds")
	}

	// Check message about successful init
	foundInitMsg := false
	for _, msg := range result.Messages {
		if strings.Contains(msg, "Chezmoi initialized successfully") {
			foundInitMsg = true
			break
		}
	}
	if !foundInitMsg {
		t.Errorf("expected 'Chezmoi initialized successfully' message, got: %v", result.Messages)
	}

	// Check that init output is captured in messages
	foundOutputMsg := false
	for _, msg := range result.Messages {
		if strings.Contains(msg, "chezmoi init output here") {
			foundOutputMsg = true
			break
		}
	}
	if !foundOutputMsg {
		t.Errorf("expected init output message, got: %v", result.Messages)
	}
}

func TestConfigure_ChezmoiFound_InitFails(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	// Create a fake chezmoi that fails on init
	binDir := createFakeChezmoiBin(t, 1, "")
	origPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", binDir+":"+origPath)
	defer func() { _ = os.Setenv("PATH", origPath) }()

	// Do NOT create chezmoi data dir — simulate first-time init that fails

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"D": "4"})

	if !result.ChezmoiInstalled {
		t.Error("ChezmoiInstalled should be true")
	}
	if result.ChezmoiInitialized {
		t.Error("ChezmoiInitialized should be false when init fails")
	}

	// Check message about init failure
	foundFailMsg := false
	for _, msg := range result.Messages {
		if strings.Contains(msg, "Chezmoi init failed") {
			foundFailMsg = true
			break
		}
	}
	if !foundFailMsg {
		t.Errorf("expected 'Chezmoi init failed' message, got: %v", result.Messages)
	}
}

func TestConfigure_ChezmoiFound_InitSucceedsNoOutput(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	// Create a fake chezmoi that succeeds on init with empty output
	binDir := createFakeChezmoiBin(t, 0, "")
	origPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", binDir+":"+origPath)
	defer func() { _ = os.Setenv("PATH", origPath) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"E": "5"})

	if !result.ChezmoiInstalled {
		t.Error("ChezmoiInstalled should be true")
	}
	if !result.ChezmoiInitialized {
		t.Error("ChezmoiInitialized should be true when init succeeds")
	}

	// Should have "Chezmoi initialized successfully" but NOT a second message with empty output
	foundInitMsg := false
	for _, msg := range result.Messages {
		if strings.Contains(msg, "Chezmoi initialized successfully") {
			foundInitMsg = true
		}
	}
	if !foundInitMsg {
		t.Errorf("expected 'Chezmoi initialized successfully' message, got: %v", result.Messages)
	}
}

// ---------------------------------------------------------------------------
// Configure — shell config write failure
// ---------------------------------------------------------------------------

func TestConfigure_ShellConfigWriteFailure(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	// Create .bashrc as a directory to cause write failure
	bashrcDir := filepath.Join(tmpDir, ".bashrc")
	if err := os.MkdirAll(bashrcDir, 0755); err != nil { //nolint:gosec
		t.Fatalf("failed to create .bashrc as directory: %v", err)
	}

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"F": "6"})

	if result.ShellConfigWritten {
		t.Error("ShellConfigWritten should be false when write fails")
	}

	// Check error message
	foundErrMsg := false
	for _, msg := range result.Messages {
		if strings.Contains(msg, "Failed to write shell config") {
			foundErrMsg = true
			break
		}
	}
	if !foundErrMsg {
		t.Errorf("expected 'Failed to write shell config' message, got: %v", result.Messages)
	}
}

// ---------------------------------------------------------------------------
// Configure — multiple env vars
// ---------------------------------------------------------------------------

func TestConfigure_MultipleEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	envVars := map[string]string{
		"VAR_A": "alpha",
		"VAR_B": "beta",
		"VAR_C": "gamma",
	}

	result := Configure(ctx, envVars)

	if !result.ShellConfigWritten {
		t.Error("ShellConfigWritten should be true")
	}
	if result.EnvVarsApplied != 3 {
		t.Errorf("EnvVarsApplied = %d, want 3", result.EnvVarsApplied)
	}

	// Verify all three vars are in the shell config
	data, err := os.ReadFile(result.ShellConfigPath)
	if err != nil {
		t.Fatalf("failed to read shell config: %v", err)
	}
	content := string(data)
	for k := range envVars {
		if !strings.Contains(content, "export "+k+"=") {
			t.Errorf("shell config should contain export for %s", k)
		}
	}
}

// ---------------------------------------------------------------------------
// Configure — comprehensive result fields check
// ---------------------------------------------------------------------------

func TestConfigure_AllResultFieldsPopulated(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/zsh")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"KEY": "val"})

	// Verify all expected fields are populated
	if result.NexusDirCreated != true {
		t.Error("NexusDirCreated should be true")
	}
	if result.NexusDir == "" {
		t.Error("NexusDir should not be empty")
	}
	if result.ShellType != "zsh" {
		t.Errorf("ShellType = %q, want 'zsh'", result.ShellType)
	}
	if result.ShellConfigPath == "" {
		t.Error("ShellConfigPath should not be empty")
	}
	if result.ShellConfigPath != filepath.Join(tmpDir, ".zshrc") {
		t.Errorf("ShellConfigPath = %q, want %q", result.ShellConfigPath, filepath.Join(tmpDir, ".zshrc"))
	}
	if result.ShellConfigWritten != true {
		t.Error("ShellConfigWritten should be true")
	}
	if result.EnvVarsApplied != 1 {
		t.Errorf("EnvVarsApplied = %d, want 1", result.EnvVarsApplied)
	}
	if len(result.Messages) == 0 {
		t.Error("Messages should not be empty")
	}
}

// ---------------------------------------------------------------------------
// injectShellConfig — edge cases for marker-based replacement
// ---------------------------------------------------------------------------

func TestInjectShellConfig_ReplaceBlockWithoutProtocolHeader(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bashrc")

	// Create a file with NEXUS_START/END markers but NO "NEXUS PROTOCOL" header.
	// This tests the branch where blockStart == -1 (falls back to startIdx).
	existing := "# some preamble\n# ─── NEXUS_START ───\nexport OLD=1\n# ─── NEXUS_END ───\n"
	if err := os.WriteFile(configPath, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	newConfig := generateBashConfig(map[string]string{"NEW": "value"})
	err := injectShellConfig(configPath, newConfig)
	if err != nil {
		t.Fatalf("injectShellConfig failed: %v", err)
	}

	data, err := os.ReadFile(configPath) //nolint:gosec
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)

	// Should have the new config
	if !strings.Contains(content, "NEXUS_START") {
		t.Error("config should contain NEXUS_START marker")
	}
	if !strings.Contains(content, "export NEW") {
		t.Error("config should contain NEW env var export")
	}

	// Old export should be gone (replaced, not appended)
	if strings.Contains(content, "export OLD") {
		t.Error("old export should have been removed during replacement")
	}

	// Should have exactly one NEXUS_START marker
	if count := strings.Count(content, "NEXUS_START"); count != 1 {
		t.Errorf("expected 1 NEXUS_START marker, got %d", count)
	}
}

func TestInjectShellConfig_ReplaceBlockWithProtocolAfterStart(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bashrc")

	// Create a file where "NEXUS PROTOCOL" appears AFTER "NEXUS_START".
	// This tests the branch where blockStart > startIdx (falls back to startIdx).
	existing := "# ─── NEXUS_START ───\n# ─── NEXUS PROTOCOL — some comment ───\nexport OLD=1\n# ─── NEXUS_END ───\n"
	if err := os.WriteFile(configPath, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	newConfig := generateBashConfig(map[string]string{"REPLACED": "yes"})
	err := injectShellConfig(configPath, newConfig)
	if err != nil {
		t.Fatalf("injectShellConfig failed: %v", err)
	}

	data, err := os.ReadFile(configPath) //nolint:gosec
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)

	// Should have the new config
	if !strings.Contains(content, "NEXUS_START") {
		t.Error("config should contain NEXUS_START marker")
	}
	if !strings.Contains(content, "export REPLACED") {
		t.Error("config should contain REPLACED env var export")
	}

	// Old export should be gone
	if strings.Contains(content, "export OLD") {
		t.Error("old export should have been removed during replacement")
	}

	// Only one NEXUS_START marker
	if count := strings.Count(content, "NEXUS_START"); count != 1 {
		t.Errorf("expected 1 NEXUS_START marker, got %d", count)
	}
}

func TestInjectShellConfig_WriteFailure(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a read-only directory to cause write failure
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	if err := os.MkdirAll(readOnlyDir, 0555); err != nil { //nolint:gosec
		t.Fatalf("failed to create read-only dir: %v", err)
	}
	configPath := filepath.Join(readOnlyDir, ".bashrc")

	config := generateBashConfig(map[string]string{"X": "y"})
	err := injectShellConfig(configPath, config)
	if err == nil {
		t.Error("expected error when writing to read-only directory")
	}
}

func TestInjectShellConfig_ExistingFileWithoutNexusBlock(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bashrc")

	// Existing config without any Nexus markers
	existing := "alias gs='git status'\nexport PATH=$HOME/bin:$PATH\n"
	if err := os.WriteFile(configPath, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	config := generateBashConfig(map[string]string{"MY_VAR": "test"})
	err := injectShellConfig(configPath, config)
	if err != nil {
		t.Fatalf("injectShellConfig failed: %v", err)
	}

	data, err := os.ReadFile(configPath) //nolint:gosec
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	// Original content should be preserved
	if !strings.Contains(content, "alias gs='git status'") {
		t.Error("original content should be preserved")
	}
	// New nexus block should be appended
	if !strings.Contains(content, "NEXUS_START") {
		t.Error("config should contain NEXUS_START marker")
	}
	if !strings.Contains(content, "export MY_VAR") {
		t.Error("config should contain MY_VAR export")
	}
}

// ---------------------------------------------------------------------------
// generateZshConfig / generateBashConfig — thorough content checks
// ---------------------------------------------------------------------------

func TestGenerateZshConfig_EmptyEnvVars(t *testing.T) {
	t.Parallel()

	output := generateZshConfig(nil)

	if !strings.Contains(output, "NEXUS_START") {
		t.Error("zsh config missing NEXUS_START marker")
	}
	if !strings.Contains(output, "NEXUS_END") {
		t.Error("zsh config missing NEXUS_END marker")
	}
	if !strings.Contains(output, "NEXUS_HOME") {
		t.Error("zsh config missing NEXUS_HOME")
	}
	if !strings.Contains(output, "HISTSIZE") {
		t.Error("zsh config missing HISTSIZE")
	}
	if !strings.Contains(output, "SAVEHIST") {
		t.Error("zsh config missing SAVEHIST")
	}
	if !strings.Contains(output, "SHARE_HISTORY") {
		t.Error("zsh config missing SHARE_HISTORY option")
	}
	if !strings.Contains(output, "HIST_IGNORE_DUPS") {
		t.Error("zsh config missing HIST_IGNORE_DUPS option")
	}
	if !strings.Contains(output, "bindkey") {
		t.Error("zsh config missing key bindings")
	}
	if !strings.Contains(output, "NEXUS_PROMPT") {
		t.Error("zsh config missing NEXUS_PROMPT")
	}
	// Zsh-specific: should NOT have PS1, should have PROMPT
	if strings.Contains(output, "PS1=") {
		t.Error("zsh config should use PROMPT, not PS1")
	}
	// Should contain common aliases
	for _, alias := range []string{"alias ll=", "alias gs=", "alias nprobe="} {
		if !strings.Contains(output, alias) {
			t.Errorf("zsh config missing %q", alias)
		}
	}
}

func TestGenerateBashConfig_EmptyEnvVars(t *testing.T) {
	t.Parallel()

	output := generateBashConfig(nil)

	if !strings.Contains(output, "NEXUS_START") {
		t.Error("bash config missing NEXUS_START marker")
	}
	if !strings.Contains(output, "NEXUS_END") {
		t.Error("bash config missing NEXUS_END marker")
	}
	if !strings.Contains(output, "NEXUS_HOME") {
		t.Error("bash config missing NEXUS_HOME")
	}
	if !strings.Contains(output, "HISTSIZE") {
		t.Error("bash config missing HISTSIZE")
	}
	if !strings.Contains(output, "HISTFILESIZE") {
		t.Error("bash config missing HISTFILESIZE")
	}
	if !strings.Contains(output, "HISTCONTROL") {
		t.Error("bash config missing HISTCONTROL")
	}
	if !strings.Contains(output, "histappend") {
		t.Error("bash config missing histappend")
	}
	if !strings.Contains(output, "PROMPT_COMMAND") {
		t.Error("bash config missing PROMPT_COMMAND")
	}
	// Bash-specific: should have PS1, not PROMPT=
	if !strings.Contains(output, "PS1=") {
		t.Error("bash config should have PS1")
	}
	// Should contain common aliases
	for _, alias := range []string{"alias ll=", "alias gs=", "alias nprobe="} {
		if !strings.Contains(output, alias) {
			t.Errorf("bash config missing %q", alias)
		}
	}
}

func TestGenerateZshConfig_WithMultipleEnvVars(t *testing.T) {
	t.Parallel()

	envVars := map[string]string{
		"ZEBRA":  "last",
		"ALPHA":  "first",
		"MIDDLE": "mid",
	}

	output := generateZshConfig(envVars)

	// Env vars should be sorted alphabetically
	alphaIdx := strings.Index(output, "export ALPHA")
	middleIdx := strings.Index(output, "export MIDDLE")
	zebraIdx := strings.Index(output, "export ZEBRA")

	if alphaIdx >= middleIdx {
		t.Error("ALPHA should appear before MIDDLE in zsh config")
	}
	if middleIdx >= zebraIdx {
		t.Error("MIDDLE should appear before ZEBRA in zsh config")
	}
}

func TestGenerateBashConfig_WithMultipleEnvVars(t *testing.T) {
	t.Parallel()

	envVars := map[string]string{
		"ZEBRA":  "last",
		"ALPHA":  "first",
		"MIDDLE": "mid",
	}

	output := generateBashConfig(envVars)

	// Env vars should be sorted alphabetically
	alphaIdx := strings.Index(output, "export ALPHA")
	middleIdx := strings.Index(output, "export MIDDLE")
	zebraIdx := strings.Index(output, "export ZEBRA")

	if alphaIdx >= middleIdx {
		t.Error("ALPHA should appear before MIDDLE in bash config")
	}
	if middleIdx >= zebraIdx {
		t.Error("MIDDLE should appear before ZEBRA in bash config")
	}
}

// ---------------------------------------------------------------------------
// Configure — shell config content verification
// ---------------------------------------------------------------------------

func TestConfigure_BashShellConfigContent(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"MY_ENV": "test_val"})
	if !result.ShellConfigWritten {
		t.Fatal("ShellConfigWritten should be true")
	}

	data, err := os.ReadFile(result.ShellConfigPath)
	if err != nil {
		t.Fatalf("failed to read .bashrc: %v", err)
	}

	content := string(data)

	// Verify bash-specific config elements
	if !strings.Contains(content, "NEXUS PROTOCOL") {
		t.Error("bash config should contain 'NEXUS PROTOCOL' header")
	}
	if !strings.Contains(content, "NEXUS_HOME") {
		t.Error("bash config should set NEXUS_HOME")
	}
	if !strings.Contains(content, "export MY_ENV=\"test_val\"") {
		t.Error("bash config should contain MY_ENV export")
	}
	if !strings.Contains(content, "alias ll=") {
		t.Error("bash config should contain aliases")
	}
}

func TestConfigure_ZshShellConfigContent(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/zsh")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"ZSH_VAR": "zsh_val"})
	if !result.ShellConfigWritten {
		t.Fatal("ShellConfigWritten should be true")
	}

	data, err := os.ReadFile(result.ShellConfigPath)
	if err != nil {
		t.Fatalf("failed to read .zshrc: %v", err)
	}

	content := string(data)

	// Verify zsh-specific config elements
	if !strings.Contains(content, "NEXUS PROTOCOL") {
		t.Error("zsh config should contain 'NEXUS PROTOCOL' header")
	}
	if !strings.Contains(content, "NEXUS_HOME") {
		t.Error("zsh config should set NEXUS_HOME")
	}
	if !strings.Contains(content, "export ZSH_VAR=\"zsh_val\"") {
		t.Error("zsh config should contain ZSH_VAR export")
	}
	if !strings.Contains(content, "bindkey") {
		t.Error("zsh config should contain key bindings")
	}
}

func TestConfigure_ExistingShellConfigPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	// Create an existing .bashrc with user content
	bashrcPath := filepath.Join(tmpDir, ".bashrc")
	existingContent := "# My custom aliases\nalias cls='clear'\nexport MY_CUSTOM='hello'\n"
	if err := os.WriteFile(bashrcPath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("failed to create .bashrc: %v", err)
	}

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"NEXUS_VAR": "nexus_val"})
	if !result.ShellConfigWritten {
		t.Fatal("ShellConfigWritten should be true")
	}

	data, err := os.ReadFile(result.ShellConfigPath)
	if err != nil {
		t.Fatalf("failed to read .bashrc: %v", err)
	}

	content := string(data)

	// User content should be preserved
	if !strings.Contains(content, "alias cls='clear'") {
		t.Error("user aliases should be preserved")
	}
	if !strings.Contains(content, "export MY_CUSTOM='hello'") {
		t.Error("user env vars should be preserved")
	}
	// Nexus content should be appended
	if !strings.Contains(content, "NEXUS_START") {
		t.Error("nexus config should be appended")
	}
	if !strings.Contains(content, "export NEXUS_VAR") {
		t.Error("nexus env var should be in config")
	}
}

func TestConfigure_IdempotentReRun(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	// First run
	result1 := Configure(ctx, map[string]string{"V1": "first"})
	if !result1.ShellConfigWritten {
		t.Fatal("first run: ShellConfigWritten should be true")
	}

	// Second run with different env vars
	result2 := Configure(ctx, map[string]string{"V2": "second"})
	if !result2.ShellConfigWritten {
		t.Fatal("second run: ShellConfigWritten should be true")
	}

	// Read the resulting config
	data, err := os.ReadFile(result2.ShellConfigPath)
	if err != nil {
		t.Fatalf("failed to read .bashrc: %v", err)
	}
	content := string(data)

	// Should have only one NEXUS_START marker (idempotent replacement)
	if count := strings.Count(content, "NEXUS_START"); count != 1 {
		t.Errorf("expected 1 NEXUS_START marker after re-run, got %d", count)
	}

	// Should have the new env var
	if !strings.Contains(content, "export V2") {
		t.Error("config should contain V2 after re-run")
	}
}

func TestConfigure_ChezmoiNotFound_MessagesAndFields(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/zsh")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	// Ensure no chezmoi on PATH
	origPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", tmpDir)
	defer func() { _ = os.Setenv("PATH", origPath) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"K": "v"})

	// Verify the complete set of result fields for the chezmoi-not-found case
	if result.ChezmoiInstalled {
		t.Error("ChezmoiInstalled should be false")
	}
	if result.ChezmoiInitialized {
		t.Error("ChezmoiInitialized should be false when chezmoi not installed")
	}
	if result.ShellType != "zsh" {
		t.Errorf("ShellType = %q, want 'zsh'", result.ShellType)
	}
	if !result.NexusDirCreated {
		t.Error("NexusDirCreated should still be true")
	}
	if !result.ShellConfigWritten {
		t.Error("ShellConfigWritten should still be true")
	}
	if result.EnvVarsApplied != 1 {
		t.Errorf("EnvVarsApplied = %d, want 1", result.EnvVarsApplied)
	}
}

// ---------------------------------------------------------------------------
// generateEnvExports — additional edge cases
// ---------------------------------------------------------------------------

func TestGenerateEnvExports_MixedSafeAndUnsafe(t *testing.T) {
	t.Parallel()

	envVars := map[string]string{
		"SAFE1":     "hello",
		"DANGEROUS": "value;rm -rf /",
		"SAFE2":     "world",
	}

	output := generateEnvExports(envVars)

	if !strings.Contains(output, "export SAFE1=\"hello\"") {
		t.Error("should export SAFE1")
	}
	if !strings.Contains(output, "export SAFE2=\"world\"") {
		t.Error("should export SAFE2")
	}
	if !strings.Contains(output, "WARNING") {
		t.Error("should contain WARNING for DANGEROUS")
	}
	if strings.Contains(output, "export DANGEROUS") {
		t.Error("should NOT export DANGEROUS value")
	}
}

// ---------------------------------------------------------------------------
// Configure — NexusDir creation failure
// ---------------------------------------------------------------------------

// TestConfigure_NexusDirCreationFailure verifies that when MkdirAll fails
// (e.g., because a file exists at the .nexus path), the error is captured
// in the result messages and NexusDirCreated is false.
func TestConfigure_NexusDirCreationFailure(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	// Create a FILE at the .nexus path so MkdirAll will fail
	nexusFile := filepath.Join(tmpDir, ".nexus")
	if err := os.WriteFile(nexusFile, []byte("blocker"), 0644); err != nil {
		t.Fatalf("failed to create blocker file: %v", err)
	}

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"X": "y"})

	if result.NexusDirCreated {
		t.Error("NexusDirCreated should be false when MkdirAll fails")
	}
	if result.NexusDir != "" {
		t.Errorf("NexusDir should be empty when creation fails, got %q", result.NexusDir)
	}

	// Check that the failure message is present
	foundFailMsg := false
	for _, msg := range result.Messages {
		if strings.Contains(msg, "Failed to create") && strings.Contains(msg, ".nexus") {
			foundFailMsg = true
			break
		}
	}
	if !foundFailMsg {
		t.Errorf("expected 'Failed to create .nexus' message, got: %v", result.Messages)
	}
}

// ---------------------------------------------------------------------------
// Configure — default shell overrides ShellType
// ---------------------------------------------------------------------------

// TestConfigure_DefaultShellOverridesType verifies that when an unknown
// shell is detected, the default case in Configure sets ShellType to "bash"
// explicitly (not just relying on detectShell).
func TestConfigure_DefaultShellOverridesType(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/usr/bin/tcsh")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"A": "1"})

	// ShellType should be overridden to "bash" in the default case
	if result.ShellType != "bash" {
		t.Errorf("ShellType = %q, want 'bash' for unknown shell", result.ShellType)
	}
	// Config should still be written to .bashrc
	if result.ShellConfigPath != filepath.Join(tmpDir, ".bashrc") {
		t.Errorf("ShellConfigPath = %q, want .bashrc path", result.ShellConfigPath)
	}
}

// ---------------------------------------------------------------------------
// generateZshConfig / generateBashConfig — nil envVars
// ---------------------------------------------------------------------------

// TestGenerateZshConfig_NilEnvVars verifies that generateZshConfig works
// with nil envVars (produces config without any env exports).
func TestGenerateZshConfig_NilEnvVars(t *testing.T) {
	t.Parallel()

	output := generateZshConfig(nil)

	if !strings.Contains(output, "NEXUS_START") {
		t.Error("zsh config should contain NEXUS_START marker even with nil envVars")
	}
	if !strings.Contains(output, "NEXUS_END") {
		t.Error("zsh config should contain NEXUS_END marker even with nil envVars")
	}
	if !strings.Contains(output, "NEXUS_HOME") {
		t.Error("zsh config should contain NEXUS_HOME even with nil envVars")
	}
}

// TestGenerateBashConfig_NilEnvVars verifies that generateBashConfig works
// with nil envVars (produces config without any env exports).
func TestGenerateBashConfig_NilEnvVars(t *testing.T) {
	t.Parallel()

	output := generateBashConfig(nil)

	if !strings.Contains(output, "NEXUS_START") {
		t.Error("bash config should contain NEXUS_START marker even with nil envVars")
	}
	if !strings.Contains(output, "NEXUS_END") {
		t.Error("bash config should contain NEXUS_END marker even with nil envVars")
	}
	if !strings.Contains(output, "NEXUS_HOME") {
		t.Error("bash config should contain NEXUS_HOME even with nil envVars")
	}
}

// ---------------------------------------------------------------------------
// generateZshConfig / generateBashConfig — content completeness
// ---------------------------------------------------------------------------

// TestGenerateZshConfig_CompleteContent verifies all key features in zsh config.
func TestGenerateZshConfig_CompleteContent(t *testing.T) {
	t.Parallel()

	output := generateZshConfig(map[string]string{"PROFILE": "dev"})

	// Verify key zsh-specific features
	zshFeatures := []string{"PROMPT=", "SHARE_HISTORY", "HIST_IGNORE_DUPS", "bindkey"}
	for _, feat := range zshFeatures {
		if !strings.Contains(output, feat) {
			t.Errorf("zsh config missing feature: %s", feat)
		}
	}
}

// TestGenerateBashConfig_CompleteContent verifies all key features in bash config.
func TestGenerateBashConfig_CompleteContent(t *testing.T) {
	t.Parallel()

	output := generateBashConfig(map[string]string{"PROFILE": "dev"})

	// Verify key bash-specific features
	bashFeatures := []string{"PS1=", "HISTCONTROL", "histappend", "PROMPT_COMMAND"}
	for _, feat := range bashFeatures {
		if !strings.Contains(output, feat) {
			t.Errorf("bash config missing feature: %s", feat)
		}
	}
}

// ---------------------------------------------------------------------------
// injectShellConfig — malformed markers edge case
// ---------------------------------------------------------------------------

// TestInjectShellConfig_MalformedMarkers_NEXUSENDBeforeNEXUSSTART verifies
// the edge case where NEXUS_END appears before NEXUS_START in the file.
// This tests the branch where endIdx <= startIdx, so the old block is NOT
// removed — the new config is simply appended.
func TestInjectShellConfig_MalformedMarkers_NEXUSENDBeforeNEXUSSTART(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bashrc")

	// Create a file where NEXUS_END comes BEFORE NEXUS_START
	existing := "# ─── NEXUS_END ───\nexport OLD=1\n# ─── NEXUS_START ───\nexport FOO=bar\n"
	if err := os.WriteFile(configPath, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	newConfig := generateBashConfig(map[string]string{"NEW": "value"})
	err := injectShellConfig(configPath, newConfig)
	if err != nil {
		t.Fatalf("injectShellConfig failed: %v", err)
	}

	data, err := os.ReadFile(configPath) //nolint:gosec
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)

	// Should have the new config appended
	if !strings.Contains(content, "NEXUS_START") {
		t.Error("config should contain NEXUS_START marker")
	}
	if !strings.Contains(content, "export NEW") {
		t.Error("config should contain NEW env var export")
	}
}

// ---------------------------------------------------------------------------
// injectShellConfig — read failure on existing file
// ---------------------------------------------------------------------------

// TestInjectShellConfig_ExistingFileUnreadable tests the path where reading
// the existing file fails (e.g., directory as file). Since os.ReadFile fails,
// existing is set to "", and the new config is written.
func TestInjectShellConfig_ExistingFileIsDir(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bashrc")

	// Create a directory at the config path (cannot be read as file)
	if err := os.MkdirAll(configPath, 0755); err != nil { //nolint:gosec // test directory
		t.Fatalf("failed to create .bashrc as directory: %v", err)
	}

	config := generateBashConfig(map[string]string{"X": "y"})
	err := injectShellConfig(configPath, config)
	// This should fail because we can't write to a path that's a directory
	if err == nil {
		t.Error("expected error when config path is a directory")
	}
}

// ---------------------------------------------------------------------------
// ConfigureResult — JSON round-trip
// ---------------------------------------------------------------------------

// TestConfigureResult_JSONRoundTrip verifies that a ConfigureResult can be
// serialized and deserialized through JSON with all fields preserved.
func TestConfigureResult_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := &ConfigureResult{
		ChezmoiInstalled:   true,
		ChezmoiInitialized: false,
		ShellConfigWritten: true,
		ShellConfigPath:    "/home/user/.zshrc",
		ShellType:          "zsh",
		NexusDirCreated:    true,
		NexusDir:           "/home/user/.nexus",
		EnvVarsApplied:     5,
		Messages:           []string{"Created /home/user/.nexus", "Chezmoi not found"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded ConfigureResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.ChezmoiInstalled != original.ChezmoiInstalled {
		t.Errorf("ChezmoiInstalled mismatch: got %v, want %v", decoded.ChezmoiInstalled, original.ChezmoiInstalled)
	}
	if decoded.ChezmoiInitialized != original.ChezmoiInitialized {
		t.Errorf("ChezmoiInitialized mismatch: got %v, want %v", decoded.ChezmoiInitialized, original.ChezmoiInitialized)
	}
	if decoded.ShellConfigWritten != original.ShellConfigWritten {
		t.Errorf("ShellConfigWritten mismatch: got %v, want %v", decoded.ShellConfigWritten, original.ShellConfigWritten)
	}
	if decoded.ShellConfigPath != original.ShellConfigPath {
		t.Errorf("ShellConfigPath mismatch: got %q, want %q", decoded.ShellConfigPath, original.ShellConfigPath)
	}
	if decoded.ShellType != original.ShellType {
		t.Errorf("ShellType mismatch: got %q, want %q", decoded.ShellType, original.ShellType)
	}
	if decoded.NexusDirCreated != original.NexusDirCreated {
		t.Errorf("NexusDirCreated mismatch: got %v, want %v", decoded.NexusDirCreated, original.NexusDirCreated)
	}
	if decoded.NexusDir != original.NexusDir {
		t.Errorf("NexusDir mismatch: got %q, want %q", decoded.NexusDir, original.NexusDir)
	}
	if decoded.EnvVarsApplied != original.EnvVarsApplied {
		t.Errorf("EnvVarsApplied mismatch: got %d, want %d", decoded.EnvVarsApplied, original.EnvVarsApplied)
	}
	if len(decoded.Messages) != len(original.Messages) {
		t.Errorf("Messages length mismatch: got %d, want %d", len(decoded.Messages), len(original.Messages))
	}
}

// TestConfigureResult_JSONZeroValues verifies JSON serialization of a
// zero-value ConfigureResult.
func TestConfigureResult_JSONZeroValues(t *testing.T) {
	t.Parallel()

	result := &ConfigureResult{
		Messages: []string{},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// All fields should be present even with zero values
	if _, ok := raw["chezmoi_installed"]; !ok {
		t.Error("JSON missing chezmoi_installed field")
	}
	if _, ok := raw["shell_config_written"]; !ok {
		t.Error("JSON missing shell_config_written field")
	}
}

// ---------------------------------------------------------------------------
// Configure — Zsh with write failure
// ---------------------------------------------------------------------------

// TestConfigure_ZshWriteFailure verifies that when the .zshrc path cannot
// be written (e.g., it's a directory), the error is properly captured.
func TestConfigure_ZshWriteFailure(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/zsh")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	// Create .zshrc as a directory to cause write failure
	zshrcDir := filepath.Join(tmpDir, ".zshrc")
	if err := os.MkdirAll(zshrcDir, 0755); err != nil {
		t.Fatalf("failed to create .zshrc as directory: %v", err)
	}

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{"Z": "val"})

	if result.ShellConfigWritten {
		t.Error("ShellConfigWritten should be false when .zshrc write fails")
	}
	if result.ShellType != "zsh" {
		t.Errorf("ShellType should still be 'zsh', got %q", result.ShellType)
	}
	if result.ShellConfigPath != filepath.Join(tmpDir, ".zshrc") {
		t.Errorf("ShellConfigPath should still be set to .zshrc path")
	}

	foundErrMsg := false
	for _, msg := range result.Messages {
		if strings.Contains(msg, "Failed to write shell config") {
			foundErrMsg = true
			break
		}
	}
	if !foundErrMsg {
		t.Errorf("expected 'Failed to write shell config' message, got: %v", result.Messages)
	}
}

// ---------------------------------------------------------------------------
// Configure — empty env vars still writes shell config
// ---------------------------------------------------------------------------

// TestConfigure_EmptyEnvVarsStillWritesConfig verifies that even with empty
// env vars, the shell config template (with NEXUS_HOME, aliases, etc.) is
// still written.
func TestConfigure_EmptyEnvVarsStillWritesConfig(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	result := Configure(ctx, map[string]string{})

	if !result.ShellConfigWritten {
		t.Error("ShellConfigWritten should be true even with empty env vars")
	}
	if result.EnvVarsApplied != 0 {
		t.Errorf("EnvVarsApplied = %d, want 0", result.EnvVarsApplied)
	}

	// Verify the shell config file has the basic nexus template
	data, err := os.ReadFile(result.ShellConfigPath)
	if err != nil {
		t.Fatalf("failed to read shell config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "NEXUS_HOME") {
		t.Error("shell config should contain NEXUS_HOME even with empty env vars")
	}
	if !strings.Contains(content, "alias ll=") {
		t.Error("shell config should contain aliases even with empty env vars")
	}
}

// ---------------------------------------------------------------------------
// Configure — mixed safe and unsafe env vars
// ---------------------------------------------------------------------------

// TestConfigure_MixedSafeAndUnsafeEnvVars verifies that Configure handles
// a mix of safe and unsafe env vars correctly: safe ones get exported,
// unsafe ones get WARNING comments, and EnvVarsApplied counts all.
func TestConfigure_MixedSafeAndUnsafeEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	origShell := os.Getenv("SHELL")
	_ = os.Setenv("SHELL", "/bin/bash")
	defer func() { _ = os.Setenv("SHELL", origShell) }()

	ctx, cancel := contextWithTimeout()
	defer cancel()

	envVars := map[string]string{
		"SAFE_A":   "hello",
		"UNSAFE_B": "value;rm -rf /",
		"SAFE_C":   "world",
	}

	result := Configure(ctx, envVars)

	if !result.ShellConfigWritten {
		t.Error("ShellConfigWritten should be true")
	}
	// EnvVarsApplied counts all env vars passed, not just the safe ones
	if result.EnvVarsApplied != 3 {
		t.Errorf("EnvVarsApplied = %d, want 3", result.EnvVarsApplied)
	}

	data, err := os.ReadFile(result.ShellConfigPath)
	if err != nil {
		t.Fatalf("failed to read shell config: %v", err)
	}
	content := string(data)

	// Safe vars should be exported
	if !strings.Contains(content, "export SAFE_A=\"hello\"") {
		t.Error("shell config should export SAFE_A")
	}
	if !strings.Contains(content, "export SAFE_C=\"world\"") {
		t.Error("shell config should export SAFE_C")
	}
	// Unsafe var should trigger WARNING
	if !strings.Contains(content, "WARNING") {
		t.Error("shell config should contain WARNING for unsafe var")
	}
}

// ---------------------------------------------------------------------------
// injectShellConfig — normal replacement with proper NEXUS PROTOCOL header
// ---------------------------------------------------------------------------

// TestInjectShellConfig_NormalReplacementWithProtocolHeader verifies the
// standard replacement path where the NEXUS PROTOCOL header is found before
// NEXUS_START, so blockStart < startIdx and the entire block including
// the header is replaced.
func TestInjectShellConfig_NormalReplacementWithProtocolHeader(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".bashrc")

	// First injection creates a proper block with the PROTOCOL header
	config1 := generateBashConfig(map[string]string{"OLD": "1"})
	if err := injectShellConfig(configPath, config1); err != nil {
		t.Fatalf("first injectShellConfig failed: %v", err)
	}

	// Verify the PROTOCOL header is present
	data1, _ := os.ReadFile(configPath)
	if !strings.Contains(string(data1), "NEXUS PROTOCOL") {
		t.Fatal("first injection should create NEXUS PROTOCOL header")
	}

	// Second injection should replace the entire block including the header
	config2 := generateBashConfig(map[string]string{"NEW": "2"})
	if err := injectShellConfig(configPath, config2); err != nil {
		t.Fatalf("second injectShellConfig failed: %v", err)
	}

	data2, _ := os.ReadFile(configPath)
	content := string(data2)

	// Should have the new env var
	if !strings.Contains(content, "export NEW") {
		t.Error("should contain NEW env var export after replacement")
	}

	// Old env var should be gone
	if strings.Contains(content, "export OLD") {
		t.Error("OLD env var should be removed after replacement")
	}

	// Should have exactly one NEXUS_START and one NEXUS_END
	if count := strings.Count(content, "NEXUS_START"); count != 1 {
		t.Errorf("expected 1 NEXUS_START marker, got %d", count)
	}
	if count := strings.Count(content, "NEXUS_END"); count != 1 {
		t.Errorf("expected 1 NEXUS_END marker, got %d", count)
	}
}
