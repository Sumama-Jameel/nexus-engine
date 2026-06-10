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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// ---------------------------------------------------------------------------
// DefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Profile != "base-dev" {
		t.Errorf("Profile = %q, want %q", cfg.Profile, "base-dev")
	}
	if cfg.PackageManager != "auto" {
		t.Errorf("PackageManager = %q, want %q", cfg.PackageManager, "auto")
	}
	if cfg.Shell != "auto" {
		t.Errorf("Shell = %q, want %q", cfg.Shell, "auto")
	}
	if cfg.AutoUpdate != false {
		t.Error("AutoUpdate should be false by default")
	}
	if cfg.Verbose != false {
		t.Error("Verbose should be false by default")
	}
}

// ---------------------------------------------------------------------------
// FormatConfig
// ---------------------------------------------------------------------------

func TestFormatConfig(t *testing.T) {
	cfg := &NexusConfig{
		Profile:        "data-science",
		PackageManager: "apt",
		Shell:          "zsh",
		AutoUpdate:     true,
	}
	output := FormatConfig(cfg)
	if !strings.Contains(output, "data-science") {
		t.Error("output should contain profile name")
	}
	if !strings.Contains(output, "apt") {
		t.Error("output should contain package manager")
	}
	if !strings.Contains(output, "zsh") {
		t.Error("output should contain shell")
	}
	if !strings.Contains(output, "true") {
		t.Error("output should contain auto_update value")
	}
}

func TestFormatConfig_Defaults(t *testing.T) {
	cfg := DefaultConfig()
	output := FormatConfig(cfg)
	if !strings.Contains(output, "base-dev") {
		t.Error("output should contain default profile")
	}
	if !strings.Contains(output, "auto") {
		t.Error("output should contain default package manager")
	}
}

// ---------------------------------------------------------------------------
// InitConfig — tests with isolated HOME directories
// ---------------------------------------------------------------------------

func TestInitConfig_CreatesDefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)                      //nolint:errcheck // test setup
	defer func() { _ = os.Setenv("HOME", origHome) }() //nolint:errcheck // test cleanup

	// Reset viper state
	viper.Reset()

	cfg, err := InitConfig()
	if err != nil {
		t.Fatalf("InitConfig failed: %v", err)
	}
	if cfg.Profile != "base-dev" {
		t.Errorf("Profile = %q, want %q", cfg.Profile, "base-dev")
	}
	if cfg.PackageManager != "auto" {
		t.Errorf("PackageManager = %q, want %q", cfg.PackageManager, "auto")
	}
}

func TestInitConfig_ReadsExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)                      //nolint:errcheck // test setup
	defer func() { _ = os.Setenv("HOME", origHome) }() //nolint:errcheck // test cleanup

	// Create nexus config directory and file
	nexusDir := filepath.Join(tmpDir, ".nexus")
	if err := os.MkdirAll(nexusDir, 0755); err != nil { //nolint:gosec
		t.Fatalf("failed to create nexus dir: %v", err)
	}
	configContent := `profile: custom-profile
package_manager: pacman
shell: bash
auto_update: true
verbose: true
`
	configPath := filepath.Join(nexusDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil { //nolint:gosec
		t.Fatalf("failed to write config: %v", err)
	}

	// Reset viper and point it to our config
	viper.Reset()
	viper.SetConfigFile(configPath)

	cfg, err := InitConfig()
	if err != nil {
		t.Fatalf("InitConfig failed: %v", err)
	}
	if cfg.Profile != "custom-profile" {
		t.Errorf("Profile = %q, want %q", cfg.Profile, "custom-profile")
	}
	if cfg.PackageManager != "pacman" {
		t.Errorf("PackageManager = %q, want %q", cfg.PackageManager, "pacman")
	}
	if cfg.Shell != "bash" {
		t.Errorf("Shell = %q, want %q", cfg.Shell, "bash")
	}
	if cfg.AutoUpdate != true {
		t.Error("AutoUpdate should be true")
	}
	if cfg.Verbose != true {
		t.Error("Verbose should be true")
	}
}

func TestInitConfig_DefaultsWhenNoConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)                      //nolint:errcheck // test setup
	defer func() { _ = os.Setenv("HOME", origHome) }() //nolint:errcheck // test cleanup

	viper.Reset()

	cfg, err := InitConfig()
	if err != nil {
		t.Fatalf("InitConfig failed: %v", err)
	}

	// Should return defaults
	if cfg.Profile != "base-dev" {
		t.Errorf("Profile = %q, want %q", cfg.Profile, "base-dev")
	}
	if cfg.AutoUpdate != false {
		t.Error("AutoUpdate should default to false")
	}
}

// ---------------------------------------------------------------------------
// createDefaultConfigFile
// ---------------------------------------------------------------------------

func TestCreateDefaultConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")

	if err := createDefaultConfigFile(path); err != nil {
		t.Fatalf("createDefaultConfigFile failed: %v", err)
	}

	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		t.Fatalf("failed to read created config: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "base-dev") {
		t.Error("default config should contain 'base-dev' profile")
	}
	if !strings.Contains(content, "package_manager: auto") {
		t.Error("default config should contain 'package_manager: auto'")
	}
	if !strings.Contains(content, "shell: auto") {
		t.Error("default config should contain 'shell: auto'")
	}
}

func TestCreateDefaultConfigFile_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "subdir", "config.yaml")

	// This should fail because the directory doesn't exist
	err := createDefaultConfigFile(path)
	if err == nil {
		t.Error("expected error when parent directory doesn't exist")
	}
}

// ---------------------------------------------------------------------------
// configFromViper
// ---------------------------------------------------------------------------

func TestConfigFromViper(t *testing.T) {
	viper.Reset()
	viper.Set("profile", "test-profile")
	viper.Set("package_manager", "dnf")
	viper.Set("shell", "fish")
	viper.Set("auto_update", true)
	viper.Set("verbose", true)

	cfg := configFromViper()
	if cfg.Profile != "test-profile" {
		t.Errorf("Profile = %q, want %q", cfg.Profile, "test-profile")
	}
	if cfg.PackageManager != "dnf" {
		t.Errorf("PackageManager = %q, want %q", cfg.PackageManager, "dnf")
	}
	if cfg.Shell != "fish" {
		t.Errorf("Shell = %q, want %q", cfg.Shell, "fish")
	}
	if cfg.AutoUpdate != true {
		t.Error("AutoUpdate should be true")
	}
	if cfg.Verbose != true {
		t.Error("Verbose should be true")
	}
}

// ---------------------------------------------------------------------------
// NexusConfig struct — field verification
// ---------------------------------------------------------------------------

func TestNexusConfig_Fields(t *testing.T) {
	cfg := &NexusConfig{
		Profile:        "test",
		PackageManager: "apk",
	}
	if cfg.Profile != "test" {
		t.Errorf("Profile = %q, want %q", cfg.Profile, "test")
	}
	if cfg.PackageManager != "apk" {
		t.Errorf("PackageManager = %q, want %q", cfg.PackageManager, "apk")
	}
}
