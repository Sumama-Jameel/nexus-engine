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
	// InstalledAt is the UTC timestamp when the package was successfully
	// installed by the Nexus engine.
	InstalledAt time.Time `json:"installed_at"`
	// Profile is the name of the Nexus profile that triggered the installation,
	// used to determine ownership during profile removal.
	Profile string `json:"profile"`
	// Verified indicates whether the package's installation was confirmed by
	// a post-install verification check (e.g., checking the binary exists on PATH).
	Verified bool `json:"verified"`
	// PackageManager is the name of the package manager used to install this
	// package (e.g., "apt-get", "dnf", "pacman"), recorded to ensure the
	// correct manager is used for removal.
	PackageManager string `json:"package_manager"`
}

// WSLInstanceState records the state of a Nexus-managed WSL2 instance.
// Per V5 "The Instant Linux Importer (The Bridge)": we track which WSL2
// distributions were imported by Nexus so that:
//  1. `nexus wsl remove` only removes Nexus-managed distros (safety)
//  2. `nexus wsl list` shows only our distros (clarity)
//  3. We can detect drift (user uninstalled a distro outside Nexus)
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

// DotfilesState records the V7/V8 state of the user's dotfile environment.
// Per V7 "The Chezmoi Integration (The Memory)" and V8 "The Git-Sync Engine
// (The Cloud)": we track the user's Chezmoi installation, binding, and
// sync history so subsequent operations can reason about drift, source
// provenance, applied-file history, and cloud-sync state.
type DotfilesState struct {
	// Installed reports whether Chezmoi is installed (via Nexus).
	Installed bool `json:"installed"`
	// Version is the parsed Chezmoi semver from the last successful detect.
	Version string `json:"version,omitempty"`
	// InstalledAt is the UTC timestamp of the Chezmoi install via Nexus.
	InstalledAt time.Time `json:"installed_at,omitempty"`
	// Source is the URL of the bound dotfile repository (V7 Slice 3+).
	// HTTPS or SSH scheme. Empty when no source has been bound.
	Source string `json:"source,omitempty"`
	// InitializedAt is the UTC timestamp when the source was bound.
	InitializedAt time.Time `json:"initialized_at,omitempty"`
	// LastAppliedAt is the UTC timestamp of the most recent apply.
	LastAppliedAt time.Time `json:"last_applied_at,omitempty"`
	// ManagedFiles is the list of paths tracked via `nexus dotfiles add`.
	ManagedFiles []string `json:"managed_files,omitempty"`
	// LastPushedAt is the UTC timestamp of the most recent successful push
	// (V8). Zero when the user has never pushed.
	LastPushedAt time.Time `json:"last_pushed_at,omitempty"`
	// LastPulledAt is the UTC timestamp of the most recent successful pull
	// (V8). Zero when the user has never pulled.
	LastPulledAt time.Time `json:"last_pulled_at,omitempty"`
	// LastCommitSHA is the SHA of the HEAD commit in the chezmoi source dir
	// after the last successful push or pull (V8). Empty when no sync has
	// happened or the user has not yet committed.
	LastCommitSHA string `json:"last_commit_sha,omitempty"`
	// Vault is the V9 secrets-vault state. Zero-valued when the vault
	// has never been initialized.
	Vault VaultState `json:"vault,omitempty"`
}

// VaultState records the V9 secrets-vault state.
//
// Per V9 "The Secrets Vault (The Shield)": the vault encrypts sensitive
// files (SSH keys, GPG keys, cloud credentials) with age encryption
// before they enter the chezmoi source dir. The private key lives at
// KeyPath (0600 permissions) and is optionally mirrored to the OS
// keyring (VaultKeyringID entry) for cross-machine recovery.
type VaultState struct {
	// Initialized reports whether `vault init` has been run successfully.
	Initialized bool `json:"initialized"`
	// PublicKey is the age public key (age1...) used to encrypt files.
	// Safe to share; not secret. Empty when not initialized.
	PublicKey string `json:"public_key,omitempty"`
	// PublicKeyShort is the first 16 chars of PublicKey, for display
	// in audit logs and status output without leaking the full key.
	PublicKeyShort string `json:"public_key_short,omitempty"`
	// KeyPath is the absolute path to the private key file.
	// Typically ~/.nexus/vault/private.key with 0600 permissions.
	KeyPath string `json:"key_path,omitempty"`
	// KeyringID is the OS keyring entry name where the private key is
	// optionally mirrored. Empty when keyring mirroring failed or was
	// not attempted. The actual keyring backend (Secret Service,
	// Keychain, Credential Manager) is platform-specific.
	KeyringID string `json:"keyring_id,omitempty"`
	// CreatedAt is the UTC timestamp of the successful `vault init`.
	CreatedAt time.Time `json:"created_at,omitempty"`
	// EncryptedFiles maps the original filesystem path to its encrypted
	// counterpart inside the chezmoi source dir.
	// e.g., "~/.ssh/id_rsa" -> "~/.local/share/chezmoi/id_rsa.age"
	EncryptedFiles map[string]string `json:"encrypted_files,omitempty"`
}

// DotfilesSyncStatus is a snapshot of the current sync posture, computed
// on demand (not persisted). Surfaced in `nexus dotfiles status` and the
// JSON output of push/pull/sync.
// ModeState tracks the V11 mode switcher state.
//
// Per ADR 010: Active is the most recently applied mode (empty before any
// apply). Previous is the mode that was active immediately before the
// current one — the target of `nexus mode rollback`. History is a bounded
// ring of the last 20 applied mode names, oldest first, for `nexus mode
// list --history` style inspection.
// ContainerState tracks a Nexus-managed Distrobox container.
//
// Per ADR 011: every container created via Nexus is tracked here
// so that `nexus container remove` only removes our containers.
type ContainerState struct {
	Name          string    `json:"name"`
	Image         string    `json:"image"`
	Family        string    `json:"family"`
	CreatedAt     time.Time `json:"created_at"`
	LastEnteredAt time.Time `json:"last_entered_at,omitempty"`
}

// HardwareReport records a single hardware + outcome snapshot (V13).
// DeviceFingerprint is SHA256(OS+Arch+CPUModel+CPUCores+GPU+RAMTotalMB)
// — NOT PII. No hostname, no IP, no serial numbers.
type HardwareReport struct {
	DeviceFingerprint string    `json:"device_fingerprint"`
	OS                string    `json:"os"`
	Arch              string    `json:"arch"`
	Kernel            string    `json:"kernel"`
	CPUModel          string    `json:"cpu_model"`
	CPUCores          int       `json:"cpu_cores"`
	RAMTotalMB        int       `json:"ram_total_mb"`
	DiskTotalGB       float64   `json:"disk_total_gb"`
	GPU               string    `json:"gpu"`
	IsWSL2            bool      `json:"is_wsl2"`
	PackageManager    string    `json:"package_manager"`
	Success           bool      `json:"success"`
	ErrorMessage      string    `json:"error_message,omitempty"`
	ProfileName       string    `json:"profile_name"`
	RecordedAt        time.Time `json:"recorded_at"`
}

// MaxLedgerRecords is the capacity of the bounded ring in HardwareLedger.
const MaxLedgerRecords = 100

// HardwareLedger tracks hardware+outcome records (V13 The Intelligence).
// Records is a bounded ring — oldest entries drop when full.
// CommunitySyncEnabled defaults to false — no data leaves the machine
// without explicit user consent.
type HardwareLedger struct {
	Records              []HardwareReport `json:"records"`
	LastAnalyzedAt       time.Time        `json:"last_analyzed_at,omitempty"`
	CommunitySyncEnabled bool             `json:"community_sync_enabled"`
	LastSyncedAt         time.Time        `json:"last_synced_at,omitempty"`
}

type ModeState struct {
	// Active is the name of the currently applied mode. Empty when no
	// mode has ever been applied on this machine.
	Active string `json:"active,omitempty"`
	// Previous is the mode that was active immediately before Active.
	// Empty on first-ever apply or after a fresh state file.
	Previous string `json:"previous,omitempty"`
	// LastSwitchAt is the UTC timestamp of the most recent successful
	// mode apply.
	LastSwitchAt time.Time `json:"last_switch_at,omitempty"`
	// LastSwitchFrom mirrors Previous for display convenience.
	LastSwitchFrom string `json:"last_switch_from,omitempty"`
	// LastSwitchTo mirrors Active for display convenience.
	LastSwitchTo string `json:"last_switch_to,omitempty"`
	// History is the bounded ring (last 20) of applied mode names,
	// oldest first. New entries are appended; the oldest is dropped
	// when the ring exceeds 20.
	History []string `json:"history,omitempty"`
}

type DotfilesSyncStatus struct {
	// Source is the bound source URL (empty when unbound).
	Source string `json:"source"`
	// LastPushedAt is the timestamp of the most recent successful push.
	LastPushedAt time.Time `json:"last_pushed_at,omitempty"`
	// LastPulledAt is the timestamp of the most recent successful pull.
	LastPulledAt time.Time `json:"last_pulled_at,omitempty"`
	// LastCommitSHA is the HEAD commit SHA of the chezmoi source dir.
	LastCommitSHA string `json:"last_commit_sha,omitempty"`
}

// NexusState is the top-level structure for the persistent state file at
// ~/.nexus/state.json. It tracks all Nexus-managed packages, applied profiles,
// and WSL2 instances to enable idempotent operations and drift detection.
type NexusState struct {
	// Version is the state file schema version. Incremented when the structure
	// changes to enable migration of existing state files.
	Version int `json:"version"`
	// LastModified is the UTC timestamp of the most recent state mutation,
	// updated on every Record* call.
	LastModified time.Time `json:"last_modified"`
	// Packages maps package names to their installation state. The key is the
	// package name as recognized by the system package manager.
	Packages map[string]PackageState `json:"packages"`
	// ProfilesApplied is the ordered list of profile names that have been
	// applied to this system, tracked to support profile-level operations.
	ProfilesApplied []string `json:"profiles_applied"`
	// WSLInstances maps WSL2 distribution names to their import state.
	// Omitted from JSON when empty via omitempty.
	WSLInstances map[string]WSLInstanceState `json:"wsl_instances,omitempty"`
	// Dotfiles tracks the V7 dotfile management state (Chezmoi install,
	// source binding, applied files). Zero-valued for legacy state files.
	Dotfiles DotfilesState `json:"dotfiles"`
	// Mode tracks the V11 mode switcher state. Zero-valued for legacy
	// state files — the zero ModeState{} is the correct "no mode ever
	// applied" representation, so no migration is required.
	Mode ModeState `json:"mode"`
	// Containers tracks V12 Distrobox containers.
	// Omitted from JSON when empty via omitempty.
	Containers map[string]ContainerState `json:"containers,omitempty"`
	// Ledger tracks V13 hardware compatibility records.
	// Zero-valued on fresh installs — Records is nil, not empty slice.
	Ledger HardwareLedger `json:"ledger"`
	// Teleported indicates whether V14 user-folder migration has been run.
	// Defaults to false on fresh installs — set to true on first successful
	// `nexus teleport` to allow status queries without re-running.
	Teleported bool `json:"teleported"`
}

// StateTracker manages the persistent installation state at ~/.nexus/state.json.
// Per the Nexus Protocol: "Immutable Infrastructure — state management."
// The state file tracks what Nexus installed so it can make intelligent
// decisions: skip already-installed packages, detect drift, enable rollback.
type StateTracker struct {
	path  string
	state *NexusState
	mu    sync.Mutex
}

// NewStateTracker creates or loads the state file.
func NewStateTracker() (*StateTracker, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine home directory: %w", err)
	}

	nexusDir := filepath.Join(homeDir, ".nexus")
	if err := os.MkdirAll(nexusDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create nexus directory %s: %w", nexusDir, err)
	}

	path, err := resolveInNexusDir(nexusDir, "state.json")
	if err != nil {
		return nil, fmt.Errorf("failed to resolve state file path: %w", err)
	}
	tracker := &StateTracker{path: path}

	// Load existing state or create new
	if data, err := os.ReadFile(path); err == nil {
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
		Mode:            ModeState{},
		Containers:      make(map[string]ContainerState),
		Dotfiles:        DotfilesState{},
		Ledger:          HardwareLedger{},
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

// RecordDotfilesInstall records that Chezmoi was installed via Nexus.
// Called by `nexus dotfiles install` after a successful Orchestrator run.
// Preserves any existing Source/InitializedAt/LastAppliedAt/ManagedFiles.
func (s *StateTracker) RecordDotfilesInstall(version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Dotfiles.Installed = true
	s.state.Dotfiles.Version = version
	s.state.Dotfiles.InstalledAt = time.Now().UTC()
	s.state.LastModified = time.Now().UTC()

	return s.save()
}

// GetDotfilesState returns a copy of the current dotfiles state.
func (s *StateTracker) GetDotfilesState() DotfilesState {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.state.Dotfiles
}

// IsDotfilesInstalled reports whether Nexus previously installed Chezmoi.
func (s *StateTracker) IsDotfilesInstalled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.state.Dotfiles.Installed
}

// RecordDotfilesInit records that a dotfile source repository was bound.
// Preserves the existing Version/InstalledAt fields.
func (s *StateTracker) RecordDotfilesInit(source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Dotfiles.Source = source
	s.state.Dotfiles.InitializedAt = time.Now().UTC()
	s.state.LastModified = time.Now().UTC()

	return s.save()
}

// RecordDotfilesApply records that dotfiles were applied from the bound source.
// Preserves Source/InitializedAt; updates LastAppliedAt and ManagedFiles.
func (s *StateTracker) RecordDotfilesApply(managedFiles []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Dotfiles.LastAppliedAt = time.Now().UTC()
	s.state.Dotfiles.ManagedFiles = managedFiles
	s.state.LastModified = time.Now().UTC()

	return s.save()
}

// RecordDotfilesAdd appends a single managed file path to the dotfiles state.
func (s *StateTracker) RecordDotfilesAdd(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.state.Dotfiles.ManagedFiles {
		if existing == path {
			// Idempotent: already tracked, no-op.
			return nil
		}
	}
	s.state.Dotfiles.ManagedFiles = append(s.state.Dotfiles.ManagedFiles, path)
	s.state.LastModified = time.Now().UTC()

	return s.save()
}

// RecordDotfilesRemove clears the source binding (but keeps the install record
// and managed-files list — `nexus dotfiles remove` unbinds, not uninstalls).
func (s *StateTracker) RecordDotfilesRemove() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Dotfiles.Source = ""
	s.state.Dotfiles.InitializedAt = time.Time{}
	s.state.LastModified = time.Now().UTC()

	return s.save()
}

// RecordDotfilesPush records a successful push to the remote. Per V8:
// we store the commit SHA and timestamp so subsequent operations can
// reason about whether the local working copy matches what was pushed.
func (s *StateTracker) RecordDotfilesPush(commitSHA string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Dotfiles.LastPushedAt = time.Now().UTC()
	if commitSHA != "" {
		s.state.Dotfiles.LastCommitSHA = commitSHA
	}
	s.state.LastModified = time.Now().UTC()

	return s.save()
}

// RecordDotfilesPull records a successful pull from the remote. Per V8:
// we store the new HEAD commit SHA and timestamp so subsequent pushes
// know whether they have diverged from the remote.
func (s *StateTracker) RecordDotfilesPull(commitSHA string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Dotfiles.LastPulledAt = time.Now().UTC()
	if commitSHA != "" {
		s.state.Dotfiles.LastCommitSHA = commitSHA
	}
	s.state.LastModified = time.Now().UTC()

	return s.save()
}

// GetDotfilesSyncStatus returns a copy of the current sync-relevant state.
// Used by `nexus dotfiles status` and the JSON output of push/pull/sync.
func (s *StateTracker) GetDotfilesSyncStatus() DotfilesSyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()

	return DotfilesSyncStatus{
		Source:        s.state.Dotfiles.Source,
		LastPushedAt:  s.state.Dotfiles.LastPushedAt,
		LastPulledAt:  s.state.Dotfiles.LastPulledAt,
		LastCommitSHA: s.state.Dotfiles.LastCommitSHA,
	}
}

// RecordVaultInit records a successful `vault init`.
//
// Initializes the EncryptedFiles map if nil and sets all the vault
// identity fields. Callers must have already generated the key pair
// and verified file permissions before calling this method.
func (s *StateTracker) RecordVaultInit(publicKey, keyPath, keyringID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Dotfiles.Vault.Initialized = true
	s.state.Dotfiles.Vault.PublicKey = publicKey
	s.state.Dotfiles.Vault.PublicKeyShort = ShortKeyFingerprint(publicKey)
	s.state.Dotfiles.Vault.KeyPath = keyPath
	s.state.Dotfiles.Vault.KeyringID = keyringID
	s.state.Dotfiles.Vault.CreatedAt = time.Now().UTC()
	if s.state.Dotfiles.Vault.EncryptedFiles == nil {
		s.state.Dotfiles.Vault.EncryptedFiles = make(map[string]string)
	}
	s.state.LastModified = time.Now().UTC()

	return s.save()
}

// RecordVaultAdd records a successfully encrypted file.
//
// Maps originalPath -> encryptedPath in the vault state so `vault list`
// can show the user what's tracked. Idempotent: re-adding an existing
// entry updates the encrypted path without creating duplicates.
func (s *StateTracker) RecordVaultAdd(originalPath, encryptedPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.Dotfiles.Vault.EncryptedFiles == nil {
		s.state.Dotfiles.Vault.EncryptedFiles = make(map[string]string)
	}
	s.state.Dotfiles.Vault.EncryptedFiles[originalPath] = encryptedPath
	s.state.LastModified = time.Now().UTC()

	return s.save()
}

// RecordVaultRemove clears a single entry from the vault tracking map.
// Does NOT delete the encrypted file from disk — callers must do that
// after this method returns success.
func (s *StateTracker) RecordVaultRemove(originalPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.state.Dotfiles.Vault.EncryptedFiles, originalPath)
	s.state.LastModified = time.Now().UTC()

	return s.save()
}

// GetVaultState returns a copy of the current vault state.
func (s *StateTracker) GetVaultState() VaultState {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.state.Dotfiles.Vault
}

// RecordModeApply persists a successful mode switch. `from` is the mode
// that was active immediately before (may be empty on first apply); `to`
// is the mode that is now active. History is updated as a bounded ring
// of the last 20 mode names.
//
// Idempotency: calling RecordModeApply twice with the same `to` is safe
// — History grows monotonically and LastSwitchAt updates each call.
func (s *StateTracker) RecordModeApply(from, to string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	s.state.Mode.Active = to
	s.state.Mode.Previous = from
	s.state.Mode.LastSwitchAt = now
	s.state.Mode.LastSwitchFrom = from
	s.state.Mode.LastSwitchTo = to

	// Bounded ring: drop the oldest entry once we exceed 20.
	const maxHistory = 20
	s.state.Mode.History = append(s.state.Mode.History, to)
	if len(s.state.Mode.History) > maxHistory {
		s.state.Mode.History = s.state.Mode.History[len(s.state.Mode.History)-maxHistory:]
	}

	s.state.LastModified = now
	return s.save()
}

// GetActiveMode returns the name of the currently active mode, or "" if
// no mode has ever been applied on this machine.
func (s *StateTracker) GetActiveMode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Mode.Active
}

// GetModeState returns a copy of the current mode state. The returned
// struct is independent of the internal state — callers may mutate it
// freely without affecting persistence.
func (s *StateTracker) GetModeState() ModeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Mode
}

// RecordContainerCreate persists a contain container creation in state.
func (s *StateTracker) RecordContainerCreate(name, image, family string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Containers == nil {
		s.state.Containers = make(map[string]ContainerState)
	}
	s.state.Containers[name] = ContainerState{
		Name:      name,
		Image:     image,
		Family:    family,
		CreatedAt: time.Now().UTC(),
	}
	s.state.LastModified = time.Now().UTC()
	return s.save()
}

// RecordContainerRemove deletes a container from state.
func (s *StateTracker) RecordContainerRemove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state.Containers, name)
	s.state.LastModified = time.Now().UTC()
	return s.save()
}

// RecordContainerEnter updates the last-entered timestamp for a container.
func (s *StateTracker) RecordContainerEnter(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Containers != nil {
		if c, ok := s.state.Containers[name]; ok {
			c.LastEnteredAt = time.Now().UTC()
			s.state.Containers[name] = c
		}
	}
	s.state.LastModified = time.Now().UTC()
	return s.save()
}

// GetContainers returns the containers map from state.
func (s *StateTracker) GetContainers() map[string]ContainerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]ContainerState, len(s.state.Containers))
	for k, v := range s.state.Containers {
		result[k] = v
	}
	return result
}

// GetContainerNames returns the names of all managed containers.
func (s *StateTracker) GetContainerNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.state.Containers))
	for name := range s.state.Containers {
		names = append(names, name)
	}
	return names
}

// IsContainerManaged checks if a container is tracked in state.
func (s *StateTracker) IsContainerManaged(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.state.Containers[name]
	return ok
}

// RecordLedgerEntry appends a HardwareReport to the bounded ring.
// When the ring exceeds MaxLedgerRecords, the oldest entry is dropped.
func (s *StateTracker) RecordLedgerEntry(report HardwareReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Ledger.Records = append(s.state.Ledger.Records, report)
	if len(s.state.Ledger.Records) > MaxLedgerRecords {
		s.state.Ledger.Records = s.state.Ledger.Records[len(s.state.Ledger.Records)-MaxLedgerRecords:]
	}
	s.state.Ledger.LastAnalyzedAt = time.Now().UTC()
	s.state.LastModified = time.Now().UTC()
	return s.save()
}

// GetLedger returns a copy of the current HardwareLedger state.
func (s *StateTracker) GetLedger() HardwareLedger {
	s.mu.Lock()
	defer s.mu.Unlock()

	records := make([]HardwareReport, len(s.state.Ledger.Records))
	copy(records, s.state.Ledger.Records)
	return HardwareLedger{
		Records:              records,
		LastAnalyzedAt:       s.state.Ledger.LastAnalyzedAt,
		CommunitySyncEnabled: s.state.Ledger.CommunitySyncEnabled,
		LastSyncedAt:         s.state.Ledger.LastSyncedAt,
	}
}

// SetCommunitySyncEnabled toggles the community sync opt-in flag.
// No data leaves the machine until the user explicitly enables this.
func (s *StateTracker) SetCommunitySyncEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Ledger.CommunitySyncEnabled = enabled
	s.state.LastModified = time.Now().UTC()
	return s.save()
}

// RecordLedgerSync updates the last-synced timestamp after a successful
// push or pull to/from the community registry.
func (s *StateTracker) RecordLedgerSync() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Ledger.LastSyncedAt = time.Now().UTC()
	s.state.LastModified = time.Now().UTC()
	return s.save()
}

// RecordTeleported marks the V14 teleport migration as completed.
// Called once after a successful `nexus teleport` to persistently track
// that user documents have been linked.
func (s *StateTracker) RecordTeleported() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.Teleported = true
	s.state.LastModified = time.Now().UTC()
	return s.save()
}

// IsTeleported reports whether the V14 teleport has been completed.
func (s *StateTracker) IsTeleported() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Teleported
}

// ShortKeyFingerprint returns the first 16 characters of an age public key,
// safe for audit logs and status display. The full key is 62 chars; the
// first 16 are enough for visual identification without leaking the full
// recipient identifier. Exported so the dotfiles package can use it for
// vault audit logging.
func ShortKeyFingerprint(pub string) string {
	if len(pub) <= 16 {
		return pub
	}
	return pub[:16]
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
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	// Atomic rename (POSIX guarantees this is atomic)
	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath) // Clean up temp file
		return fmt.Errorf("failed to commit state: %w", err)
	}

	return nil
}
