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
        "fmt"
        "os"
        "path/filepath"
        "sync"
        "time"
)

// PackageState records the installation state of a single package managed
// by Nexus. It persists to ~/.nexus/state.json and enables idempotent
// operations, drift detection, and targeted rollback.
type PackageState struct {
        // Profile is the name of the Nexus profile that triggered the installation,
        // used to determine ownership during profile removal.
        Profile string `json:"profile"`
        // PackageManager is the name of the package manager used to install this
        // package (e.g., "apt-get", "dnf", "pacman"), recorded to ensure the
        // correct manager is used for removal.
        PackageManager string `json:"package_manager"`
        // Verified indicates whether the package's installation was confirmed by
        // a post-install verification check (e.g., checking the binary exists on PATH).
        Verified bool `json:"verified"`
        // InstalledAt is the UTC timestamp when the package was successfully
        // installed by the Nexus engine.
        InstalledAt time.Time `json:"installed_at"`
}

// WSLInstanceState records the state of a Nexus-managed WSL2 instance.
// Per V5 "The Instant Linux Importer (The Bridge)": we track which WSL2
// distributions were imported by Nexus so that:
//   1. `nexus wsl remove` only removes Nexus-managed distros (safety)
//   2. `nexus wsl list` shows only our distros (clarity)
//   3. We can detect drift (user uninstalled a distro outside Nexus)
type WSLInstanceState struct {
        // ImageName is the human-readable name of the imported WSL2 distribution,
        // used as the key in the WSLInstances map and as the WSL --name parameter.
        ImageName string `json:"image_name"`
        // ImageVersion is the version tag of the imported rootfs tarball
        // (e.g., "22.04" for Ubuntu 22.04).
        ImageVersion string `json:"image_version"`
        // TarballSHA256 is the SHA-256 hash of the original rootfs tarball,
        // stored to detect tampering or corruption of the source image.
        TarballSHA256 string `json:"tarball_sha256"`
        // InstallPath is the filesystem directory where the WSL2 instance's
        // virtual disk was extracted and registered.
        InstallPath string `json:"install_path"`
        // Family is the distribution family identifier (e.g., "debian", "redhat",
        // "arch"), used to select the correct package manager inside the instance.
        Family string `json:"family"`
        // ImportedAt is the UTC timestamp when the WSL2 instance was imported.
        ImportedAt time.Time `json:"imported_at"`
}

// NexusState is the top-level structure for the persistent state file at
// ~/.nexus/state.json. It tracks all Nexus-managed packages, applied profiles,
// and WSL2 instances to enable idempotent operations and drift detection.
type NexusState struct {
        // Packages maps package names to their installation state. The key is the
        // package name as recognized by the system package manager.
        Packages map[string]PackageState `json:"packages"`
        // ProfilesApplied is the ordered list of profile names that have been
        // applied to this system, tracked to support profile-level operations.
        ProfilesApplied []string `json:"profiles_applied"`
        // WSLInstances maps WSL2 distribution names to their import state.
        // Omitted from JSON when empty via omitempty.
        WSLInstances map[string]WSLInstanceState `json:"wsl_instances,omitempty"`
        // LastModified is the UTC timestamp of the most recent state mutation,
        // updated on every Record* call.
        LastModified time.Time `json:"last_modified"`
        // Version is the state file schema version. Incremented when the structure
        // changes to enable migration of existing state files.
        Version int `json:"version"`
}

// StateTracker manages the persistent installation state at ~/.nexus/state.json.
// Per the Nexus Protocol: "Immutable Infrastructure — state management."
// The state file tracks what Nexus installed so it can make intelligent
// decisions: skip already-installed packages, detect drift, enable rollback.
type StateTracker struct {
        state *NexusState
        mu    sync.Mutex
        path  string
}

// NewStateTracker creates or loads the state file.
func NewStateTracker() (*StateTracker, error) {
        homeDir, err := os.UserHomeDir()
        if err != nil {
                return nil, fmt.Errorf("failed to determine home directory: %w", err)
        }

        nexusDir := filepath.Join(homeDir, ".nexus")
        os.MkdirAll(nexusDir, 0755)

        path := filepath.Join(nexusDir, "state.json")
        tracker := &StateTracker{path: path}

        // Load existing state or create new
        if data, err := os.ReadFile(path); err == nil { //nolint:gosec
                var state NexusState
                if err := json.Unmarshal(data, &state); err == nil {
                        // Migration: ensure WSLInstances is initialized for existing state files
                        if state.WSLInstances == nil {
                                state.WSLInstances = make(map[string]WSLInstanceState)
                        }
                        tracker.state = &state
                        return tracker, nil
                }
        }

        // Fresh state
        tracker.state = &NexusState{
                Version:         1,
                LastModified:    time.Now().UTC(),
                Packages:        make(map[string]PackageState),
                ProfilesApplied: []string{},
                WSLInstances:    make(map[string]WSLInstanceState),
        }

        return tracker, nil
}

// RecordInstall adds a successfully installed package to the state.
func (s *StateTracker) RecordInstall(pkg string, profile string, pmName string, verified bool) error {
        s.mu.Lock()
        defer s.mu.Unlock()

        s.state.Packages[pkg] = PackageState{
                InstalledAt:    time.Now().UTC(),
                Profile:        profile,
                Verified:       verified,
                PackageManager: pmName,
        }
        s.state.LastModified = time.Now().UTC()

        // Track profile
        found := false
        for _, p := range s.state.ProfilesApplied {
                if p == profile {
                        found = true
                        break
                }
        }
        if !found {
                s.state.ProfilesApplied = append(s.state.ProfilesApplied, profile)
        }

        return s.save()
}

// RecordRemove removes a package from the state.
func (s *StateTracker) RecordRemove(pkg string) error {
        s.mu.Lock()
        defer s.mu.Unlock()

        delete(s.state.Packages, pkg)
        s.state.LastModified = time.Now().UTC()

        return s.save()
}

// IsManaged checks if a package is managed by Nexus.
func (s *StateTracker) IsManaged(pkg string) bool {
        s.mu.Lock()
        defer s.mu.Unlock()

        _, exists := s.state.Packages[pkg]
        return exists
}

// GetManagedPackages returns all Nexus-managed packages.
func (s *StateTracker) GetManagedPackages() map[string]PackageState {
        s.mu.Lock()
        defer s.mu.Unlock()

        // Return a copy
        result := make(map[string]PackageState, len(s.state.Packages))
        for k, v := range s.state.Packages {
                result[k] = v
        }
        return result
}

// GetProfiles returns all applied profiles.
func (s *StateTracker) GetProfiles() []string {
        s.mu.Lock()
        defer s.mu.Unlock()

        return append([]string{}, s.state.ProfilesApplied...)
}

// RecordWSLImport records a Nexus-managed WSL2 instance import.
// Per V5: we track which WSL2 distributions were imported by Nexus
// so that we can safely remove only our distros and detect drift.
func (s *StateTracker) RecordWSLImport(name, image, version, sha256, installPath, family string) error {
        s.mu.Lock()
        defer s.mu.Unlock()

        if s.state.WSLInstances == nil {
                s.state.WSLInstances = make(map[string]WSLInstanceState)
        }

        s.state.WSLInstances[name] = WSLInstanceState{
                ImageName:     image,
                ImageVersion:  version,
                TarballSHA256: sha256,
                InstallPath:   installPath,
                Family:        family,
                ImportedAt:    time.Now().UTC(),
        }
        s.state.LastModified = time.Now().UTC()

        return s.save()
}

// RecordWSLRemove removes a WSL2 instance from the state.
func (s *StateTracker) RecordWSLRemove(name string) error {
        s.mu.Lock()
        defer s.mu.Unlock()

        if s.state.WSLInstances != nil {
                delete(s.state.WSLInstances, name)
        }
        s.state.LastModified = time.Now().UTC()

        return s.save()
}

// GetWSLInstances returns all Nexus-managed WSL2 instances.
func (s *StateTracker) GetWSLInstances() map[string]WSLInstanceState {
        s.mu.Lock()
        defer s.mu.Unlock()

        result := make(map[string]WSLInstanceState, len(s.state.WSLInstances))
        for k, v := range s.state.WSLInstances {
                result[k] = v
        }
        return result
}

// IsWSLManaged checks if a WSL2 distribution is managed by Nexus.
func (s *StateTracker) IsWSLManaged(name string) bool {
        s.mu.Lock()
        defer s.mu.Unlock()

        _, exists := s.state.WSLInstances[name]
        return exists
}

// save writes the state to disk atomically.
// Atomic write: write to temp file, then rename. This prevents
// corruption if the process crashes mid-write.
func (s *StateTracker) save() error {
        data, err := json.MarshalIndent(s.state, "", "  ")
        if err != nil {
                return fmt.Errorf("failed to marshal state: %w", err)
        }

        // Write to temp file first
        tmpPath := s.path + ".tmp"
        if err := os.WriteFile(tmpPath, data, 0644); err != nil { //nolint:gosec
                return fmt.Errorf("failed to write state: %w", err)
        }

        // Atomic rename (POSIX guarantees this is atomic)
        if err := os.Rename(tmpPath, s.path); err != nil {
                _ = os.Remove(tmpPath) // Clean up temp file
                return fmt.Errorf("failed to commit state: %w", err)
        }

        return nil
}
