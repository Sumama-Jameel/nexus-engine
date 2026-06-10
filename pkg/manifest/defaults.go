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

package manifest

import (
	"embed"
	"io/fs"
	"path/filepath"
	"strings"
)

//go:embed defaults/*.yaml
var defaultsFS embed.FS

// BundledDefaults returns a map of profile-name → YAML-content for all
// profiles bundled with the binary via go:embed. These are the "factory
// defaults" that ship with every Nexus installation.
//
// Per the V3 plan: "The binary ships with defaults (embedded), but the
// user's profiles live in their home directory." Bundled defaults are
// extracted to the ProfileStore on first run via ProfileStore.Initialize.
//
// The embedded profiles are located in the defaults/ subdirectory and are
// read at compile time. Only files with the .yaml extension are included.
func BundledDefaults() map[string]string {
	defaults := make(map[string]string)

	entries, err := fs.ReadDir(defaultsFS, "defaults")
	if err != nil {
		return defaults
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}

		profileName := strings.TrimSuffix(name, ".yaml")
		data, err := defaultsFS.ReadFile("defaults/" + name)
		if err != nil {
			continue
		}

		defaults[profileName] = string(data)
	}

	return defaults
}

// DefaultRemoteBaseURL returns the configured remote base URL for profile
// fetching. By default, this returns DefaultRemoteURL. The URL can be
// overridden via Viper configuration to point to a custom profile repository.
func DefaultRemoteBaseURL() string {
	return DefaultRemoteURL
}

func init() {
	// Ensure filepath functions work with embedded FS paths
	_ = filepath.Join("defaults", "base-dev.yaml")
}
