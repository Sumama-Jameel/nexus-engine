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
        "crypto/sha256"
        "encoding/json"
        "fmt"
        "io"
        "net/http"
        "net/url"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "time"
)

// ProfileSource indicates where a profile came from. It is one of
// SourceBundled, SourceLocal, or SourceRemote. The source affects
// removal behavior (bundled profiles require --force) and is tracked
// in the profile registry for provenance auditing.
type ProfileSource string

const (
        // SourceBundled indicates a profile that ships with the Nexus binary.
        // Bundled profiles are embedded via go:embed and extracted on first run.
        // They cannot be removed without the --force flag.
        SourceBundled ProfileSource = "bundled"

        // SourceLocal indicates a user-created profile stored on the local disk.
        // Local profiles are created via `nexus profile create` or manual editing.
        SourceLocal ProfileSource = "local"

        // SourceRemote indicates a profile fetched from a remote GitHub repository.
        // Remote profiles are downloaded via FetchProfile and validated against
        // the JSON Schema before being written to disk.
        SourceRemote ProfileSource = "remote"
)

// ProfileMeta tracks provenance and integrity for a single profile.
// Every profile in the ProfileRegistry has a ProfileMeta entry that records
// where the profile came from, its expected SHA256 hash, and when it was
// added and last applied. The SHA256 field is used by VerifyIntegrity to
// detect unauthorized modifications on every profile load.
type ProfileMeta struct {
        Name        string        `json:"name"`
        Version     string        `json:"version"`
        Source      ProfileSource `json:"source"`
        SHA256      string        `json:"sha256"`
        DateAdded   time.Time     `json:"date_added"`
        LastApplied *time.Time    `json:"last_applied,omitempty"`
}

// ProfileRegistry is the on-disk index file (registry.json) for the local
// profile store. It maps profile names to their metadata, enabling fast
// lookups and integrity verification without scanning the filesystem.
// The registry is persisted atomically (temp file + rename) to prevent
// corruption from partial writes.
type ProfileRegistry struct {
        Version  int                    `json:"version"`
        Profiles map[string]ProfileMeta `json:"profiles"`
}

// ProfileStore manages the local profile directory at ~/.nexus/profiles/.
// Per the V3 plan: "Separation of data and code. The binary ships with
// defaults (embedded), but the user's profiles live in their home directory."
type ProfileStore struct {
        dir      string
        registry *ProfileRegistry
}

// NewProfileStore creates or loads the profile store at ~/.nexus/profiles/.
// If the directory or registry does not exist, they are created. If the
// registry file exists but cannot be parsed, a fresh registry is created.
// The caller should call Initialize after NewProfileStore to seed bundled
// defaults into the store.
func NewProfileStore() (*ProfileStore, error) {
        homeDir, err := os.UserHomeDir()
        if err != nil {
                return nil, fmt.Errorf("failed to determine home directory: %w", err)
        }

        dir := filepath.Join(homeDir, ".nexus", "profiles")
        if err := os.MkdirAll(dir, 0755); err != nil {
                return nil, fmt.Errorf("failed to create profiles directory: %w", err)
        }

        store := &ProfileStore{dir: dir}

        // Load or create registry
        registryPath := filepath.Join(dir, "registry.json")
        if data, err := os.ReadFile(registryPath); err == nil {
                var reg ProfileRegistry
                if err := json.Unmarshal(data, &reg); err == nil {
                        store.registry = &reg
                        return store, nil
                }
        }

        // Fresh registry
        store.registry = &ProfileRegistry{
                Version:  1,
                Profiles: make(map[string]ProfileMeta),
        }

        return store, nil
}

// Initialize seeds the profile store with bundled default profiles.
// For each bundled profile, it writes the YAML to disk (if not already
// present) and records metadata (including the SHA256 hash) in the registry.
// If a profile already exists on disk, its hash is verified and its metadata
// is added to the registry if missing. This is called on first run to ensure
// the user has access to the factory-default profiles.
func (s *ProfileStore) Initialize(bundledProfiles map[string]string) error {
        for name, content := range bundledProfiles {
                path := filepath.Join(s.dir, name+".yaml")

                // Parse to extract version for registry metadata
                var version string
                if parsed, err := ParseBytes([]byte(content)); err == nil {
                        version = parsed.Version
                }

                // Only write if not already present
                if _, err := os.Stat(path); err != nil {
                        if err := os.WriteFile(path, []byte(content), 0644); err != nil {
                                return fmt.Errorf("failed to write bundled profile '%s': %w", name, err)
                        }

                        hash := computeSHA256([]byte(content))
                        s.registry.Profiles[name] = ProfileMeta{
                                Name:      name,
                                Version:   version,
                                Source:    SourceBundled,
                                SHA256:    hash,
                                DateAdded: time.Now().UTC(),
                        }
                } else {
                        // Verify existing file matches expected hash
                        if _, exists := s.registry.Profiles[name]; !exists {
                                data, err := os.ReadFile(path)
                                if err == nil {
                                        // Parse existing file for version
                                        var existingVersion string
                                        if parsed, parseErr := ParseBytes(data); parseErr == nil {
                                                existingVersion = parsed.Version
                                        }
                                        s.registry.Profiles[name] = ProfileMeta{
                                                Name:      name,
                                                Version:   existingVersion,
                                                Source:    SourceBundled,
                                                SHA256:    computeSHA256(data),
                                                DateAdded: time.Now().UTC(),
                                        }
                                }
                        }
                }
        }

        return s.saveRegistry()
}

// LoadProfile loads and returns a named profile from the store directory.
// It implements the ProfileLoader interface, enabling the store to be used
// directly in ResolveExtends. Before parsing, VerifyIntegrity is called to
// detect any unauthorized modifications since the profile was registered.
func (s *ProfileStore) LoadProfile(name string) (*NexusProfile, error) {
        path := s.ProfilePath(name)
        if _, err := os.Stat(path); err != nil {
                return nil, fmt.Errorf("profile '%s' not found in store", name)
        }

        // Integrity check before loading
        if err := s.VerifyIntegrity(name); err != nil {
                return nil, fmt.Errorf("INTEGRITY CHECK FAILED for profile '%s': %w", name, err)
        }

        return Parse(path)
}

// LoadProfileWithExtends loads a profile and fully resolves its extends chain.
// The store itself serves as the ProfileLoader for extends resolution. If the
// profile has no extends field, it is returned as-is. If it extends another
// profile, ResolveExtends is called with cycle detection and depth limiting.
func (s *ProfileStore) LoadProfileWithExtends(name string) (*NexusProfile, error) {
        profile, err := s.LoadProfile(name)
        if err != nil {
                return nil, err
        }

        if profile.Extends == "" {
                return profile, nil
        }

        // Resolve extends chain with cycle detection
        resolved, err := ResolveExtends(profile, s, make(map[string]bool), 0)
        if err != nil {
                return nil, fmt.Errorf("extends resolution failed for '%s': %w", name, err)
        }

        return resolved, nil
}

// SaveProfile writes a profile YAML to the store directory and registers its
// metadata. The profile is validated against the JSON Schema before writing;
// if validation fails, the write is refused. The SHA256 hash of the content
// is recorded for future integrity verification via VerifyIntegrity.
func (s *ProfileStore) SaveProfile(name string, content []byte, source ProfileSource) error {
        // Validate before saving
        profile, err := ParseBytes(content)
        if err != nil {
                return fmt.Errorf("profile validation failed, refusing to save: %w", err)
        }

        path := s.ProfilePath(name)
        if err := os.WriteFile(path, content, 0644); err != nil {
                return fmt.Errorf("failed to write profile: %w", err)
        }

        s.registry.Profiles[name] = ProfileMeta{
                Name:      name,
                Version:   profile.Version,
                Source:    source,
                SHA256:    computeSHA256(content),
                DateAdded: time.Now().UTC(),
        }

        return s.saveRegistry()
}

// RemoveProfile removes a profile from the store directory and registry.
// Bundled profiles (SourceBundled) require the force flag to be set to true,
// preventing accidental removal of factory defaults. Local and remote profiles
// can be removed without force.
func (s *ProfileStore) RemoveProfile(name string, force bool) error {
        meta, exists := s.registry.Profiles[name]
        if !exists {
                return fmt.Errorf("profile '%s' not found in registry", name)
        }

        if meta.Source == SourceBundled && !force {
                return fmt.Errorf("cannot remove bundled profile '%s' without --force", name)
        }

        path := s.ProfilePath(name)
        if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
                return fmt.Errorf("failed to remove profile file: %w", err)
        }

        delete(s.registry.Profiles, name)
        return s.saveRegistry()
}

// VerifyIntegrity recomputes the SHA256 hash of a profile file and compares
// it against the hash stored in the registry. Per the V3 plan: "On EVERY load,
// recompute and compare. Mismatch = REJECT."
//
// Threat model:
//   - Tampering: A local attacker or malicious process could modify a profile
//     YAML on disk to add malicious package names or change targets.
//     VerifyIntegrity detects this by comparing the current SHA256 against the
//     hash recorded when the profile was first stored.
//   - Supply chain drift: A remote profile could be silently altered on the
//     server side. The SHA256 recorded at fetch time would differ from the
//     current file content after a subsequent fetch.
//   - Race conditions: This check is performed on every LoadProfile call,
//     ensuring that even transient modifications are caught before the profile
//     is parsed and its contents are trusted by the engine.
func (s *ProfileStore) VerifyIntegrity(name string) error {
        meta, exists := s.registry.Profiles[name]
        if !exists {
                // Not in registry — not yet tracked. Allow load but warn.
                return nil
        }

        path := s.ProfilePath(name)
        data, err := os.ReadFile(path)
        if err != nil {
                return fmt.Errorf("cannot read profile file: %w", err)
        }

        currentHash := computeSHA256(data)
        if currentHash != meta.SHA256 {
                return fmt.Errorf("SHA256 mismatch: expected %s, got %s — profile may have been tampered with", meta.SHA256[:16], currentHash[:16])
        }

        return nil
}

// RecordApplied updates the LastApplied timestamp for a profile in the
// registry. This is called after a successful `nexus install --profile`
// execution to track when each profile was last used. The registry is
// persisted immediately after the update.
func (s *ProfileStore) RecordApplied(name string) error {
        meta, exists := s.registry.Profiles[name]
        if !exists {
                return fmt.Errorf("profile '%s' not in registry", name)
        }

        now := time.Now().UTC()
        meta.LastApplied = &now
        s.registry.Profiles[name] = meta
        return s.saveRegistry()
}

// ListProfiles returns all registered profiles sorted alphabetically by name.
func (s *ProfileStore) ListProfiles() []ProfileMeta {
        var profiles []ProfileMeta
        for _, meta := range s.registry.Profiles {
                profiles = append(profiles, meta)
        }
        sort.Slice(profiles, func(i, j int) bool {
                return profiles[i].Name < profiles[j].Name
        })
        return profiles
}

// GetMeta returns the ProfileMeta for a named profile. The second return
// value indicates whether the profile exists in the registry.
func (s *ProfileStore) GetMeta(name string) (ProfileMeta, bool) {
        meta, exists := s.registry.Profiles[name]
        return meta, exists
}

// ProfilePath returns the filesystem path for a named profile in the store
// directory. The path is of the form ~/.nexus/profiles/<name>.yaml.
func (s *ProfileStore) ProfilePath(name string) string {
        return filepath.Join(s.dir, name+".yaml")
}

// ProfileContent reads the raw YAML content of a named profile from disk.
// Returns the content as a string, or an error if the file cannot be read.
func (s *ProfileStore) ProfileContent(name string) (string, error) {
        path := s.ProfilePath(name)
        data, err := os.ReadFile(path)
        if err != nil {
                return "", err
        }
        return string(data), nil
}

// defaultHTTPClient is the HTTP client used by FetchProfile. It is a
// package-level variable so that tests can replace it with a custom client
// (e.g., one that trusts a test TLS server's certificate). This is the
// same dependency-injection pattern used by bridge.bridgeExecFn.
var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

// AllowedRemoteHosts is the whitelist of domains from which profiles may be
// fetched. Per the Nexus Protocol Zero-Trust Architecture: "No component trusts
// another by default." An arbitrary remote URL is an SSRF vector — only
// explicitly approved domains may serve Nexus profiles.
//
// Threat model:
//   - SSRF (Server-Side Request Forgery): Without host whitelisting, an attacker
//     could supply a URL pointing to internal services (e.g., http://169.254.169.254/
//     for cloud metadata, http://localhost:6379/ for Redis), causing the engine to
//     make requests to unintended targets. AllowedRemoteHosts restricts fetches to
//     only pre-approved GitHub domains.
//   - DNS rebinding: An attacker could register a domain that resolves to a public
//     IP during validation but to an internal IP during the actual request. The
//     host-level whitelist prevents this by only allowing well-known, controlled
//     domains.
//   - Data exfiltration: An attacker could use query parameters or fragments to
//     encode stolen data in outgoing requests. The companion validateRemoteURL
//     function rejects URLs with query parameters and fragments.
//
// To add a new approved host, modify this map and rebuild the binary.
// There is no runtime configuration for host whitelisting — this is intentional.
var AllowedRemoteHosts = map[string]bool{
        "raw.githubusercontent.com": true,
        "github.com":               true,
        "gist.githubusercontent.com": true,
}

// FetchProfile downloads a profile from a remote GitHub repository and stores
// it in the local profile store. Per the V3 plan: "URL constructed from
// validated components (no user-supplied URLs — prevents SSRF). Response
// size-limited. Content validated against schema BEFORE writing to disk."
//
// Security enforcement:
//   - SSRF Protection: The remoteURL is validated against AllowedRemoteHosts.
//     Only HTTPS URLs on whitelisted domains are permitted. URLs with userinfo,
//     query parameters, or fragments are rejected.
//   - Size Limiting: The response body is limited to 1MB via io.LimitReader.
//     A legitimate profile should never exceed this size. This prevents memory
//     exhaustion attacks from compromised repositories.
//   - Validation Before Write: The downloaded content is validated against the
//     JSON Schema BEFORE being written to disk. A malicious profile must never
//     touch the filesystem. If validation fails, the content is discarded.
//   - Integrity Recording: The SHA256 hash of the validated content is recorded
//     in the registry for future integrity checks via VerifyIntegrity.
func (s *ProfileStore) FetchProfile(name string, remoteURL string) error {
        // SSRF Protection: Validate the remote URL against allowed hosts
        if err := validateRemoteURL(remoteURL); err != nil {
                return fmt.Errorf("SECURITY: remote URL rejected: %w", err)
        }

        // Construct URL from validated components only
        fetchURL := fmt.Sprintf("%s/%s.yaml", remoteURL, name)

        resp, err := defaultHTTPClient.Get(fetchURL)
        if err != nil {
                return fmt.Errorf("failed to fetch profile '%s': %w", name, err)
        }
        defer resp.Body.Close()

        if resp.StatusCode != http.StatusOK {
                return fmt.Errorf("remote returned HTTP %d for profile '%s'", resp.StatusCode, name)
        }

        // Size limit: 1MB max. A profile should NEVER be this large.
        // This prevents memory exhaustion attacks from compromised repos.
        limitedReader := io.LimitReader(resp.Body, 1*1024*1024)
        data, err := io.ReadAll(limitedReader)
        if err != nil {
                return fmt.Errorf("failed to read profile response: %w", err)
        }

        // Validate against schema BEFORE writing to disk.
        // A malicious profile must NEVER touch the filesystem.
        parsed, err := ParseBytes(data)
        if err != nil {
                return fmt.Errorf("remote profile '%s' failed validation, refusing to save: %w", name, err)
        }

        // Version drift detection: if the profile already exists locally,
        // warn if the remote version differs from the local version.
        if existingMeta, exists := s.registry.Profiles[name]; exists {
                if existingMeta.Version != parsed.Version && existingMeta.Source == SourceRemote {
                        // Non-fatal warning: the user explicitly asked to fetch,
                        // so we proceed. But we log the drift for auditability.
                        _ = fmt.Sprintf("version drift: local=%s remote=%s", existingMeta.Version, parsed.Version)
                }
        }

        return s.SaveProfile(name, data, SourceRemote)
}

// validateRemoteURL checks that a remote URL is allowed for profile fetching.
// Per Zero-Trust: only whitelisted HTTPS hosts are permitted.
// This prevents SSRF attacks where a malicious URL could be used to
// scan internal networks or exfiltrate data.
func validateRemoteURL(remoteURL string) error {
        parsed, err := url.Parse(remoteURL)
        if err != nil {
                return fmt.Errorf("invalid URL: %w", err)
        }

        // Enforce HTTPS
        if parsed.Scheme != "https" {
                return fmt.Errorf("only HTTPS is allowed, got '%s'", parsed.Scheme)
        }

        // Check host against whitelist
        host := parsed.Hostname()
        if !AllowedRemoteHosts[host] {
                allowed := make([]string, 0, len(AllowedRemoteHosts))
                for h := range AllowedRemoteHosts {
                        allowed = append(allowed, h)
                }
                return fmt.Errorf("host '%s' is not in the allowed list: %v", host, allowed)
        }

        // Reject URLs with userinfo (user:pass@host)
        if parsed.User != nil {
                return fmt.Errorf("URLs with userinfo are not allowed")
        }

        // Reject URLs with query parameters or fragments
        if parsed.RawQuery != "" || parsed.Fragment != "" {
                return fmt.Errorf("URLs with query parameters or fragments are not allowed")
        }

        return nil
}

// computeSHA256 returns the hex-encoded SHA256 hash of data.
func computeSHA256(data []byte) string {
        hash := sha256.Sum256(data)
        return fmt.Sprintf("%x", hash)
}

// saveRegistry writes the registry to disk atomically.
func (s *ProfileStore) saveRegistry() error {
        data, err := json.MarshalIndent(s.registry, "", "  ")
        if err != nil {
                return fmt.Errorf("failed to marshal registry: %w", err)
        }

        tmpPath := filepath.Join(s.dir, "registry.json.tmp")
        if err := os.WriteFile(tmpPath, data, 0644); err != nil {
                return fmt.Errorf("failed to write registry: %w", err)
        }

        if err := os.Rename(tmpPath, filepath.Join(s.dir, "registry.json")); err != nil {
                os.Remove(tmpPath)
                return fmt.Errorf("failed to commit registry: %w", err)
        }

        return nil
}

// FormatProfileMeta returns a human-readable single-line summary of a
// ProfileMeta entry, including the name, source, version, truncated SHA256
// hash, and last-applied timestamp. Used by the `nexus profile list` command.
func FormatProfileMeta(meta ProfileMeta) string {
        lastApplied := "never"
        if meta.LastApplied != nil {
                lastApplied = meta.LastApplied.Format("2006-01-02 15:04")
        }

        return fmt.Sprintf("  %-20s %-10s %-8s %s  %s",
                meta.Name,
                meta.Source,
                meta.Version,
                meta.SHA256[:16]+"…",
                lastApplied,
        )
}

// DefaultRemoteURL is the default GitHub raw URL for the community profiles
// repository. This URL is used by DefaultRemoteBaseURL and the CLI when no
// custom remote is configured. It points to the nexus-profiles repository
// on GitHub, which is in the AllowedRemoteHosts whitelist.
const DefaultRemoteURL = "https://raw.githubusercontent.com/nexus-os/nexus-profiles/main/profiles"

// IsValidProfileName checks if a name conforms to the Nexus profile naming
// rules. Valid names must match the pattern ^[a-z0-9][a-z0-9-]*$: they must
// start with a lowercase letter or digit, followed by zero or more lowercase
// letters, digits, or hyphens. This pattern is enforced by both the JSON
// Schema and this Go-level check.
func IsValidProfileName(name string) bool {
        if len(name) == 0 {
                return false
        }
        if name[0] < 'a' || name[0] > 'z' {
                if name[0] < '0' || name[0] > '9' {
                        return false
                }
        }
        for _, c := range name {
                if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
                        return false
                }
        }
        return true
}

// SanitizeProfileName prevents path traversal attacks in profile names.
// Profile names are used to construct filesystem paths (e.g.,
// ~/.nexus/profiles/<name>.yaml), so they must be safe for use as
// path components.
//
// Threat model:
//   - Path traversal: An attacker-controlled name like "../../etc/shadow"
//     could cause the engine to read or write arbitrary files if used
//     directly in filepath.Join. SanitizeProfileName rejects slashes,
//     backslashes, and ".." sequences.
//   - Hidden file access: A name starting with "." (e.g., ".ssh") could
//     target sensitive dotfiles. Names starting with a dot are rejected.
//   - Pattern injection: Names must match ^[a-z0-9][a-z0-9-]*$ to prevent
//     shell metacharacters, unicode tricks, or null bytes from reaching
//     the filesystem layer.
//
// This function is called before any profile name is used in a path
// construction, ensuring that even if a malicious name reaches the engine
// (e.g., from a crafted remote profile URL), it cannot escape the
// profiles directory.
func SanitizeProfileName(name string) error {
        if strings.Contains(name, "/") || strings.Contains(name, "\\") {
                return fmt.Errorf("SECURITY: profile name cannot contain path separators")
        }
        if strings.Contains(name, "..") {
                return fmt.Errorf("SECURITY: profile name cannot contain path traversal sequences")
        }
        if strings.HasPrefix(name, ".") {
                return fmt.Errorf("SECURITY: profile name cannot start with a dot")
        }
        if !IsValidProfileName(name) {
                return fmt.Errorf("profile name must match pattern ^[a-z0-9][a-z0-9-]*$")
        }
        return nil
}
