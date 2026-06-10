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
        "fmt"
        "os"
        "path/filepath"

        "github.com/spf13/viper"
)

// NexusConfig represents the persistent Nexus Engine configuration,
// loaded from YAML via Viper. Per plan.md: "cobra (CLI framework), viper (configuration)."
// The configuration is searched in a layered path: cwd, ~/.nexus, and /etc/nexus.
type NexusConfig struct {
        // Profile is the active Nexus profile name that determines which packages
        // and configurations are applied (e.g., "base-dev", "full-desktop").
        Profile string `json:"profile"`
        // PackageManager specifies the system package manager to use (e.g., "apt-get",
        // "dnf", "pacman"). A value of "auto" triggers automatic detection based on
        // the probed system.
        PackageManager string `json:"package_manager"`
        // Shell specifies the preferred shell for configuration injection ("zsh" or
        // "bash"). A value of "auto" defers to the SHELL environment variable.
        Shell string `json:"shell"`
        // AutoUpdate controls whether the engine should automatically apply package
        // updates when drift is detected during verification.
        AutoUpdate bool `json:"auto_update"`
        // Verbose enables detailed logging of engine operations for troubleshooting.
        Verbose bool `json:"verbose"`
}

// DefaultConfig returns the default Nexus configuration.
func DefaultConfig() *NexusConfig {
        return &NexusConfig{
                Profile:        "base-dev",
                PackageManager: "auto",
                Shell:          "auto",
                AutoUpdate:     false,
                Verbose:        false,
        }
}

// InitConfig initializes Viper and loads the Nexus configuration.
// It searches for .nexus.yaml in:
// 1. Current working directory
// 2. ~/.nexus/config.yaml
// 3. /etc/nexus/config.yaml
// If no config file exists, it creates one at ~/.nexus/config.yaml with defaults.
func InitConfig() (*NexusConfig, error) {
        homeDir, _ := os.UserHomeDir()
        nexusDir := filepath.Join(homeDir, ".nexus")

        // Ensure the nexus directory exists
        os.MkdirAll(nexusDir, 0755)

        viper.SetConfigName("config")
        viper.SetConfigType("yaml")

        // Add search paths in priority order
        viper.AddConfigPath(".")           // Current directory
        viper.AddConfigPath(nexusDir)      // ~/.nexus/
        viper.AddConfigPath("/etc/nexus/") // System-wide

        // Set defaults
        viper.SetDefault("profile", "base-dev")
        viper.SetDefault("package_manager", "auto")
        viper.SetDefault("shell", "auto")
        viper.SetDefault("auto_update", false)
        viper.SetDefault("verbose", false)

        // Attempt to read config
        if err := viper.ReadInConfig(); err != nil {
                if _, ok := err.(viper.ConfigFileNotFoundError); ok {
                        // No config file found — create one with defaults
                        configPath := filepath.Join(nexusDir, "config.yaml")
                        if err := createDefaultConfigFile(configPath); err != nil {
                                // Non-fatal: just use defaults without a file
                                return configFromViper(), nil
                        }
                        viper.SetConfigFile(configPath)
                        _ = viper.ReadInConfig()
                } else {
                        // Config file exists but couldn't be parsed
                        return nil, fmt.Errorf("failed to parse config: %w", err)
                }
        }

        return configFromViper(), nil
}

// configFromViper maps Viper values to our config struct.
func configFromViper() *NexusConfig {
        return &NexusConfig{
                Profile:        viper.GetString("profile"),
                PackageManager: viper.GetString("package_manager"),
                Shell:          viper.GetString("shell"),
                AutoUpdate:     viper.GetBool("auto_update"),
                Verbose:        viper.GetBool("verbose"),
        }
}

// createDefaultConfigFile writes a default config.yaml.
func createDefaultConfigFile(path string) error {
        content := `# Nexus Protocol — Engine Configuration
# This file is managed by the Nexus Engine.
# Edit manually or use: nexus config set <key> <value>

profile: base-dev
package_manager: auto
shell: auto
auto_update: false
verbose: false
`
        return os.WriteFile(path, []byte(content), 0644) //nolint:gosec
}

// FormatConfig returns a human-readable summary of the current config.
func FormatConfig(cfg *NexusConfig) string {
        return fmt.Sprintf("  Profile: %s | Pkg Manager: %s | Shell: %s | Auto Update: %v",
                cfg.Profile, cfg.PackageManager, cfg.Shell, cfg.AutoUpdate)
}
