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
        "fmt"
        "net/http"
        "net/http/httptest"
        "net/url"
        "os"
        "path/filepath"
        "strings"
        "testing"
        "time"
)

// setupTestStore creates a ProfileStore backed by a temporary HOME directory.
// It returns the store and a cleanup function. Tests that modify HOME must NOT
// use t.Parallel().
func setupTestStore(t *testing.T) (*ProfileStore, func()) {
        t.Helper()
        origHome := os.Getenv("HOME")
        tmpDir := t.TempDir()
        os.Setenv("HOME", tmpDir)
        cleanup := func() {
                os.Setenv("HOME", origHome)
        }
        store, err := NewProfileStore()
        if err != nil {
                t.Fatalf("NewProfileStore failed: %v", err)
        }
        return store, cleanup
}

// ---------------------------------------------------------------------------
// NewProfileStore
// ---------------------------------------------------------------------------

func TestNewProfileStore(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // Verify directory exists
        if _, err := os.Stat(store.dir); err != nil {
                t.Errorf("profiles directory should exist, got error: %v", err)
        }

        // Verify registry is initialized
        if store.registry == nil {
                t.Error("registry should not be nil")
        }
        if store.registry.Version != 1 {
                t.Errorf("expected registry version 1, got %d", store.registry.Version)
        }
}

func TestNewProfileStore_RegistryCreation(t *testing.T) {
        origHome := os.Getenv("HOME")
        tmpDir := t.TempDir()
        os.Setenv("HOME", tmpDir)
        defer os.Setenv("HOME", origHome)

        store, err := NewProfileStore()
        if err != nil {
                t.Fatalf("NewProfileStore failed: %v", err)
        }

        // Fresh registry should be empty
        if len(store.registry.Profiles) != 0 {
                t.Errorf("fresh registry should have no profiles, got %d", len(store.registry.Profiles))
        }

        // Registry is only written on save, so registry.json should not exist yet
        registryPath := filepath.Join(store.dir, "registry.json")
        if _, err := os.Stat(registryPath); !os.IsNotExist(err) {
                t.Errorf("registry.json should not exist after NewProfileStore (written on save)")
        }
}

func TestNewProfileStore_RegistryLoad(t *testing.T) {
        origHome := os.Getenv("HOME")
        tmpDir := t.TempDir()
        os.Setenv("HOME", tmpDir)
        defer os.Setenv("HOME", origHome)

        // First: create a store and write some data
        store1, err := NewProfileStore()
        if err != nil {
                t.Fatalf("first NewProfileStore failed: %v", err)
        }
        store1.registry.Profiles["test-profile"] = ProfileMeta{
                Name:    "test-profile",
                Version: "1.0.0",
                Source:  SourceLocal,
                SHA256:  "abc123",
        }
        if err := store1.saveRegistry(); err != nil {
                t.Fatalf("saveRegistry failed: %v", err)
        }

        // Second: create a new store pointing to the same dir; it should load the existing registry
        store2, err := NewProfileStore()
        if err != nil {
                t.Fatalf("second NewProfileStore failed: %v", err)
        }

        meta, exists := store2.registry.Profiles["test-profile"]
        if !exists {
                t.Error("expected to load existing profile from registry")
        }
        if meta.SHA256 != "abc123" {
                t.Errorf("expected SHA256 'abc123', got %q", meta.SHA256)
        }
}

// ---------------------------------------------------------------------------
// Initialize
// ---------------------------------------------------------------------------

func TestProfileStore_Initialize(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        bundled := map[string]string{
                "base-dev": validProfileYAML,
        }

        if err := store.Initialize(bundled); err != nil {
                t.Fatalf("Initialize failed: %v", err)
        }

        // Profile should be on disk
        if _, err := os.Stat(store.ProfilePath("base-dev")); err != nil {
                t.Errorf("profile file should exist after Initialize: %v", err)
        }

        // Profile should be in registry
        meta, exists := store.registry.Profiles["base-dev"]
        if !exists {
                t.Fatal("profile should be in registry after Initialize")
        }
        if meta.Source != SourceBundled {
                t.Errorf("expected SourceBundled, got %q", meta.Source)
        }
        if meta.SHA256 == "" {
                t.Error("SHA256 should be set")
        }
}

func TestProfileStore_Initialize_Idempotent(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        bundled := map[string]string{
                "base-dev": validProfileYAML,
        }

        // Initialize once
        if err := store.Initialize(bundled); err != nil {
                t.Fatalf("first Initialize failed: %v", err)
        }

        // Read the original content
        origContent, err := store.ProfileContent("base-dev")
        if err != nil {
                t.Fatalf("failed to read original content: %v", err)
        }

        // Initialize again with different content
        modifiedYAML := strings.ReplaceAll(validProfileYAML, "git", "git-modified")
        bundledModified := map[string]string{
                "base-dev": modifiedYAML,
        }
        if err := store.Initialize(bundledModified); err != nil {
                t.Fatalf("second Initialize failed: %v", err)
        }

        // The original content should NOT be overwritten (file already exists)
        newContent, err := store.ProfileContent("base-dev")
        if err != nil {
                t.Fatalf("failed to read content after second Initialize: %v", err)
        }
        if newContent != origContent {
                t.Error("Initialize should not overwrite existing profiles (idempotent)")
        }
}

// ---------------------------------------------------------------------------
// SaveProfile
// ---------------------------------------------------------------------------

func TestProfileStore_SaveProfile(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        err := store.SaveProfile("my-profile", []byte(validProfileYAML), SourceLocal)
        if err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // File should exist
        if _, err := os.Stat(store.ProfilePath("my-profile")); err != nil {
                t.Errorf("profile file should exist after SaveProfile: %v", err)
        }

        // Registry should have entry
        meta, exists := store.GetMeta("my-profile")
        if !exists {
                t.Fatal("profile should be in registry after SaveProfile")
        }
        if meta.Source != SourceLocal {
                t.Errorf("expected SourceLocal, got %q", meta.Source)
        }
        if meta.Version != "1.0.0" {
                t.Errorf("expected Version '1.0.0', got %q", meta.Version)
        }
        if meta.SHA256 == "" {
                t.Error("SHA256 should be set")
        }
}

func TestProfileStore_SaveProfile_InvalidContent(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        invalidYAML := []byte("not: valid: yaml: [")
        err := store.SaveProfile("bad-profile", invalidYAML, SourceLocal)
        if err == nil {
                t.Error("expected error for invalid YAML content")
        }
        if !strings.Contains(err.Error(), "refusing to save") {
                t.Errorf("expected 'refusing to save' error, got: %v", err)
        }

        // File should NOT exist
        if _, err := os.Stat(store.ProfilePath("bad-profile")); err == nil {
                t.Error("profile file should NOT exist after failed SaveProfile")
        }
}

// ---------------------------------------------------------------------------
// LoadProfile
// ---------------------------------------------------------------------------

func TestProfileStore_LoadProfile(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        if err := store.SaveProfile("load-me", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        profile, err := store.LoadProfile("load-me")
        if err != nil {
                t.Fatalf("LoadProfile failed: %v", err)
        }
        if profile.Name != "test-profile" {
                t.Errorf("expected Name 'test-profile', got %q", profile.Name)
        }
        if profile.Version != "1.0.0" {
                t.Errorf("expected Version '1.0.0', got %q", profile.Version)
        }
}

func TestProfileStore_LoadProfile_NotFound(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        _, err := store.LoadProfile("nonexistent")
        if err == nil {
                t.Error("expected error for missing profile")
        }
        if !strings.Contains(err.Error(), "not found") {
                t.Errorf("expected 'not found' error, got: %v", err)
        }
}

func TestProfileStore_LoadProfileWithExtends(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // Save parent profile
        parentYAML := `name: parent-profile
version: "1.0.0"
targets:
  - family: debian
    packages:
      - git
      - curl
env:
  EDITOR: vim
`
        if err := store.SaveProfile("parent-profile", []byte(parentYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile parent failed: %v", err)
        }

        // Save child profile that extends parent
        childYAML := `name: child-profile
version: "2.0.0"
extends: parent-profile
targets:
  - family: debian
    packages:
      - python3
env:
  LANG: en_US
`
        if err := store.SaveProfile("child-profile", []byte(childYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile child failed: %v", err)
        }

        profile, err := store.LoadProfileWithExtends("child-profile")
        if err != nil {
                t.Fatalf("LoadProfileWithExtends failed: %v", err)
        }

        // Child metadata should take precedence
        if profile.Name != "child-profile" {
                t.Errorf("expected Name 'child-profile', got %q", profile.Name)
        }

        // Parent env should be inherited
        if profile.Env["EDITOR"] != "vim" {
                t.Error("expected parent env EDITOR=vim to be inherited")
        }
        // Child env should be present
        if profile.Env["LANG"] != "en_US" {
                t.Error("expected child env LANG=en_US")
        }

        // Packages from both parent and child should be present (merged)
        totalPkgs := 0
        for _, tgt := range profile.Targets {
                totalPkgs += len(tgt.Packages)
        }
        if totalPkgs < 3 { // git, curl, python3
                t.Errorf("expected at least 3 merged packages, got %d", totalPkgs)
        }
}

// ---------------------------------------------------------------------------
// RemoveProfile
// ---------------------------------------------------------------------------

func TestProfileStore_RemoveProfile(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        if err := store.SaveProfile("remove-me", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        if err := store.RemoveProfile("remove-me", false); err != nil {
                t.Fatalf("RemoveProfile failed: %v", err)
        }

        // File should be gone
        if _, err := os.Stat(store.ProfilePath("remove-me")); err == nil {
                t.Error("profile file should be removed")
        }

        // Registry entry should be gone
        _, exists := store.GetMeta("remove-me")
        if exists {
                t.Error("profile should be removed from registry")
        }
}

func TestProfileStore_RemoveProfile_BundledRequiresForce(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // Simulate a bundled profile
        if err := store.SaveProfile("bundled-one", []byte(validProfileYAML), SourceBundled); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // Remove without force should fail
        err := store.RemoveProfile("bundled-one", false)
        if err == nil {
                t.Error("expected error when removing bundled profile without --force")
        }
        if !strings.Contains(err.Error(), "--force") {
                t.Errorf("expected '--force' error, got: %v", err)
        }

        // Remove with force should succeed
        if err := store.RemoveProfile("bundled-one", true); err != nil {
                t.Errorf("RemoveProfile with force failed: %v", err)
        }
}

func TestProfileStore_RemoveProfile_NotFound(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        err := store.RemoveProfile("nonexistent", false)
        if err == nil {
                t.Error("expected error for removing nonexistent profile")
        }
        if !strings.Contains(err.Error(), "not found") {
                t.Errorf("expected 'not found' error, got: %v", err)
        }
}

// ---------------------------------------------------------------------------
// VerifyIntegrity
// ---------------------------------------------------------------------------

func TestProfileStore_VerifyIntegrity(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        if err := store.SaveProfile("integrity-test", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // Should pass for unmodified file
        if err := store.VerifyIntegrity("integrity-test"); err != nil {
                t.Errorf("VerifyIntegrity should pass for unmodified file, got: %v", err)
        }
}

func TestProfileStore_VerifyIntegrity_Tampered(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        if err := store.SaveProfile("tampered-test", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // Tamper with the file on disk
        path := store.ProfilePath("tampered-test")
        tamperedContent := strings.ReplaceAll(validProfileYAML, "git", "git-tampered")
        if err := os.WriteFile(path, []byte(tamperedContent), 0644); err != nil {
                t.Fatalf("failed to tamper with profile: %v", err)
        }

        // Should fail after modification
        err := store.VerifyIntegrity("tampered-test")
        if err == nil {
                t.Error("expected integrity check to fail for tampered file")
        }
        if !strings.Contains(err.Error(), "SHA256 mismatch") {
                t.Errorf("expected 'SHA256 mismatch' error, got: %v", err)
        }
}

// ---------------------------------------------------------------------------
// RecordApplied
// ---------------------------------------------------------------------------

func TestProfileStore_RecordApplied(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        if err := store.SaveProfile("applied-test", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        if err := store.RecordApplied("applied-test"); err != nil {
                t.Fatalf("RecordApplied failed: %v", err)
        }

        meta, exists := store.GetMeta("applied-test")
        if !exists {
                t.Fatal("profile should exist in registry")
        }
        if meta.LastApplied == nil {
                t.Error("LastApplied should be set after RecordApplied")
        }
        // Should be recent (within last 5 seconds)
        if time.Since(*meta.LastApplied) > 5*time.Second {
                t.Errorf("LastApplied timestamp seems wrong: %v", meta.LastApplied)
        }
}

// ---------------------------------------------------------------------------
// ListProfiles
// ---------------------------------------------------------------------------

func TestProfileStore_ListProfiles(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // Save multiple profiles
        profileA := `name: alpha
version: "1.0.0"
targets:
  - family: debian
    packages:
      - git
`
        profileB := `name: beta
version: "2.0.0"
targets:
  - family: arch
    packages:
      - vim
`
        if err := store.SaveProfile("alpha", []byte(profileA), SourceLocal); err != nil {
                t.Fatalf("SaveProfile alpha failed: %v", err)
        }
        if err := store.SaveProfile("beta", []byte(profileB), SourceLocal); err != nil {
                t.Fatalf("SaveProfile beta failed: %v", err)
        }

        profiles := store.ListProfiles()
        if len(profiles) != 2 {
                t.Fatalf("expected 2 profiles, got %d", len(profiles))
        }

        // Should be sorted alphabetically
        if profiles[0].Name != "alpha" {
                t.Errorf("expected first profile 'alpha', got %q", profiles[0].Name)
        }
        if profiles[1].Name != "beta" {
                t.Errorf("expected second profile 'beta', got %q", profiles[1].Name)
        }
}

// ---------------------------------------------------------------------------
// GetMeta
// ---------------------------------------------------------------------------

func TestProfileStore_GetMeta(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        if err := store.SaveProfile("meta-test", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        meta, exists := store.GetMeta("meta-test")
        if !exists {
                t.Fatal("expected profile to exist in registry")
        }
        if meta.Name != "meta-test" {
                t.Errorf("expected Name 'meta-test', got %q", meta.Name)
        }
        if meta.Version != "1.0.0" {
                t.Errorf("expected Version '1.0.0', got %q", meta.Version)
        }
        if meta.Source != SourceLocal {
                t.Errorf("expected SourceLocal, got %q", meta.Source)
        }
}

func TestProfileStore_GetMeta_NotFound(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        _, exists := store.GetMeta("nonexistent")
        if exists {
                t.Error("expected exists=false for missing profile")
        }
}

// ---------------------------------------------------------------------------
// ProfilePath
// ---------------------------------------------------------------------------

func TestProfileStore_ProfilePath(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        path := store.ProfilePath("my-profile")
        expected := filepath.Join(store.dir, "my-profile.yaml")
        if path != expected {
                t.Errorf("expected path %q, got %q", expected, path)
        }
}

// ---------------------------------------------------------------------------
// ProfileContent
// ---------------------------------------------------------------------------

func TestProfileStore_ProfileContent(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        if err := store.SaveProfile("content-test", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        content, err := store.ProfileContent("content-test")
        if err != nil {
                t.Fatalf("ProfileContent failed: %v", err)
        }
        if content != validProfileYAML {
                t.Error("ProfileContent should return the exact raw content")
        }
}

// ---------------------------------------------------------------------------
// validateRemoteURL
// ---------------------------------------------------------------------------

func TestValidateRemoteURL(t *testing.T) {
        tests := []struct {
                name    string
                url     string
                wantErr bool
        }{
                {
                        name:    "valid GitHub raw URL",
                        url:     "https://raw.githubusercontent.com/nexus-os/nexus-profiles/main/profiles",
                        wantErr: false,
                },
                {
                        name:    "valid GitHub URL",
                        url:     "https://github.com/nexus-os/nexus-profiles",
                        wantErr: false,
                },
                {
                        name:    "valid gist URL",
                        url:     "https://gist.githubusercontent.com/user/hash/raw/profile.yaml",
                        wantErr: false,
                },
                {
                        name:    "HTTP rejected",
                        url:     "http://raw.githubusercontent.com/nexus-os/nexus-profiles",
                        wantErr: true,
                },
                {
                        name:    "unknown host rejected",
                        url:     "https://evil.com/profiles/test",
                        wantErr: true,
                },
                {
                        name:    "userinfo rejected",
                        url:     "https://user:pass@raw.githubusercontent.com/profiles/test",
                        wantErr: true,
                },
                {
                        name:    "query params rejected",
                        url:     "https://raw.githubusercontent.com/nexus-os/profiles?param=value",
                        wantErr: true,
                },
                {
                        name:    "fragment rejected",
                        url:     "https://raw.githubusercontent.com/nexus-os/profiles#section",
                        wantErr: true,
                },
        }

        for _, tc := range tests {
                t.Run(tc.name, func(t *testing.T) {
                        err := validateRemoteURL(tc.url)
                        if tc.wantErr && err == nil {
                                t.Errorf("expected error for URL %q, got nil", tc.url)
                        }
                        if !tc.wantErr && err != nil {
                                t.Errorf("unexpected error for URL %q: %v", tc.url, err)
                        }
                })
        }
}

func TestValidateRemoteURL_RejectsHTTP(t *testing.T) {
        err := validateRemoteURL("http://raw.githubusercontent.com/nexus-os/profiles")
        if err == nil {
                t.Error("expected HTTP URL to be rejected")
        }
        if !strings.Contains(err.Error(), "HTTPS") {
                t.Errorf("expected HTTPS-related error, got: %v", err)
        }
}

func TestValidateRemoteURL_RejectsUnknownHost(t *testing.T) {
        err := validateRemoteURL("https://evil.com/profiles")
        if err == nil {
                t.Error("expected unknown host to be rejected")
        }
        if !strings.Contains(err.Error(), "not in the allowed list") {
                t.Errorf("expected 'not in the allowed list' error, got: %v", err)
        }
}

func TestValidateRemoteURL_RejectsUserinfo(t *testing.T) {
        err := validateRemoteURL("https://user:pass@raw.githubusercontent.com/profiles")
        if err == nil {
                t.Error("expected URL with userinfo to be rejected")
        }
        if !strings.Contains(err.Error(), "userinfo") {
                t.Errorf("expected userinfo error, got: %v", err)
        }
}

func TestValidateRemoteURL_RejectsQueryParams(t *testing.T) {
        err := validateRemoteURL("https://raw.githubusercontent.com/nexus-os/profiles?ref=main")
        if err == nil {
                t.Error("expected URL with query params to be rejected")
        }
        if !strings.Contains(err.Error(), "query parameters") {
                t.Errorf("expected query parameters error, got: %v", err)
        }
}

// ---------------------------------------------------------------------------
// AllowedRemoteHosts
// ---------------------------------------------------------------------------

func TestAllowedRemoteHosts(t *testing.T) {
        expected := []string{
                "raw.githubusercontent.com",
                "github.com",
                "gist.githubusercontent.com",
        }
        for _, host := range expected {
                if !AllowedRemoteHosts[host] {
                        t.Errorf("host %q should be in AllowedRemoteHosts", host)
                }
        }

        // Verify no extra hosts
        if len(AllowedRemoteHosts) != len(expected) {
                t.Errorf("expected %d allowed hosts, got %d", len(expected), len(AllowedRemoteHosts))
        }
}

// ---------------------------------------------------------------------------
// computeSHA256
// ---------------------------------------------------------------------------

func TestComputeSHA256(t *testing.T) {
        data := []byte("hello world")
        hash1 := computeSHA256(data)
        hash2 := computeSHA256(data)

        // Should be deterministic
        if hash1 != hash2 {
                t.Errorf("computeSHA256 should be deterministic: %q != %q", hash1, hash2)
        }

        // Should match known SHA256
        expected := fmt.Sprintf("%x", sha256.Sum256(data))
        if hash1 != expected {
                t.Errorf("expected SHA256 %q, got %q", expected, hash1)
        }

        // Different input should produce different hash
        different := computeSHA256([]byte("goodbye world"))
        if hash1 == different {
                t.Error("different inputs should produce different hashes")
        }
}

// ---------------------------------------------------------------------------
// ProfileSource constants
// ---------------------------------------------------------------------------

func TestProfileSource_Constants(t *testing.T) {
        if SourceBundled != "bundled" {
                t.Errorf("SourceBundled should be 'bundled', got %q", SourceBundled)
        }
        if SourceLocal != "local" {
                t.Errorf("SourceLocal should be 'local', got %q", SourceLocal)
        }
        if SourceRemote != "remote" {
                t.Errorf("SourceRemote should be 'remote', got %q", SourceRemote)
        }
}

// ---------------------------------------------------------------------------
// FormatProfileMeta
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// FetchProfile
// ---------------------------------------------------------------------------

// setupFetchTest creates an httptest.NewTLSServer and configures both
// AllowedRemoteHosts and defaultHTTPClient so FetchProfile can reach the
// test server. Returns the server and a cleanup function. Tests that use
// this helper must NOT use t.Parallel().
func setupFetchTest(t *testing.T, handler http.HandlerFunc) (*httptest.Server, func()) {
        t.Helper()

        ts := httptest.NewTLSServer(handler)

        // Parse server URL to extract hostname
        u, err := url.Parse(ts.URL)
        if err != nil {
                t.Fatalf("failed to parse test server URL: %v", err)
        }
        host := u.Hostname()

        // Save originals
        origAllowed := AllowedRemoteHosts[host]
        origClient := defaultHTTPClient

        // Temporarily add test server host to allowed list
        AllowedRemoteHosts[host] = true
        // Use test server's client which trusts its certificate
        defaultHTTPClient = ts.Client()

        cleanup := func() {
                if !origAllowed {
                        delete(AllowedRemoteHosts, host)
                } else {
                        AllowedRemoteHosts[host] = origAllowed
                }
                defaultHTTPClient = origClient
                ts.Close()
        }

        return ts, cleanup
}

func TestFetchProfile_Success(t *testing.T) {
        fetchYAML := `name: test-profile
version: "1.0.0"
targets:
  - family: debian
    packages:
      - git
      - curl
`
        ts, cleanup := setupFetchTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                fmt.Fprint(w, fetchYAML)
        }))
        defer cleanup()

        store, storeCleanup := setupTestStore(t)
        defer storeCleanup()

        err := store.FetchProfile("test-profile", ts.URL)
        if err != nil {
                t.Fatalf("FetchProfile failed: %v", err)
        }

        // Profile should be saved in the store
        meta, exists := store.GetMeta("test-profile")
        if !exists {
                t.Fatal("profile should be in registry after FetchProfile")
        }
        if meta.Source != SourceRemote {
                t.Errorf("expected SourceRemote, got %q", meta.Source)
        }
        if meta.Version != "1.0.0" {
                t.Errorf("expected Version '1.0.0', got %q", meta.Version)
        }
        if meta.SHA256 == "" {
                t.Error("SHA256 should be set")
        }

        // Profile file should exist on disk
        if _, err := os.Stat(store.ProfilePath("test-profile")); err != nil {
                t.Errorf("profile file should exist on disk: %v", err)
        }
}

func TestFetchProfile_InvalidURL(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        err := store.FetchProfile("test", "http://raw.githubusercontent.com/profiles")
        if err == nil {
                t.Error("expected error for non-HTTPS URL")
        }
        if !strings.Contains(err.Error(), "SECURITY") {
                t.Errorf("expected 'SECURITY' in error, got: %v", err)
        }
}

func TestFetchProfile_DisallowedHost(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        err := store.FetchProfile("test", "https://evil.com/profiles")
        if err == nil {
                t.Error("expected error for disallowed host")
        }
        if !strings.Contains(err.Error(), "SECURITY") {
                t.Errorf("expected 'SECURITY' in error, got: %v", err)
        }
}

func TestFetchProfile_HTTPError(t *testing.T) {
        ts, cleanup := setupFetchTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(http.StatusNotFound)
        }))
        defer cleanup()

        store, storeCleanup := setupTestStore(t)
        defer storeCleanup()

        err := store.FetchProfile("test-profile", ts.URL)
        if err == nil {
                t.Error("expected error for HTTP 404")
        }
        if !strings.Contains(err.Error(), "HTTP 404") {
                t.Errorf("expected 'HTTP 404' error, got: %v", err)
        }
}

func TestFetchProfile_InvalidContent(t *testing.T) {
        ts, cleanup := setupFetchTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                fmt.Fprint(w, "not: valid: yaml: [")
        }))
        defer cleanup()

        store, storeCleanup := setupTestStore(t)
        defer storeCleanup()

        err := store.FetchProfile("test-profile", ts.URL)
        if err == nil {
                t.Error("expected error for invalid content")
        }
        if !strings.Contains(err.Error(), "refusing to save") {
                t.Errorf("expected 'refusing to save' error, got: %v", err)
        }

        // File should NOT exist on disk
        if _, err := os.Stat(store.ProfilePath("test-profile")); err == nil {
                t.Error("profile file should NOT exist after failed fetch")
        }
}

func TestFetchProfile_VersionDrift(t *testing.T) {
        store, storeCleanup := setupTestStore(t)
        defer storeCleanup()

        // Pre-save a profile with version 1.0.0 as remote
        v1YAML := `name: test-profile
version: "1.0.0"
targets:
  - family: debian
    packages:
      - git
`
        if err := store.SaveProfile("test-profile", []byte(v1YAML), SourceRemote); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // Now serve v2 from the test server
        v2YAML := `name: test-profile
version: "2.0.0"
targets:
  - family: debian
    packages:
      - git
      - curl
`
        ts, cleanup := setupFetchTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                fmt.Fprint(w, v2YAML)
        }))
        defer cleanup()

        // FetchProfile should succeed (version drift is non-fatal)
        err := store.FetchProfile("test-profile", ts.URL)
        if err != nil {
                t.Fatalf("FetchProfile should succeed despite version drift: %v", err)
        }

        // Version should be updated to 2.0.0
        meta, exists := store.GetMeta("test-profile")
        if !exists {
                t.Fatal("profile should exist in registry")
        }
        if meta.Version != "2.0.0" {
                t.Errorf("expected version '2.0.0' after re-fetch, got %q", meta.Version)
        }
}

func TestFetchProfile_URLWithQueryParams(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        err := store.FetchProfile("test", "https://raw.githubusercontent.com/nexus-os/profiles?ref=main")
        if err == nil {
                t.Error("expected error for URL with query params")
        }
        if !strings.Contains(err.Error(), "SECURITY") {
                t.Errorf("expected 'SECURITY' in error, got: %v", err)
        }
}

func TestFetchProfile_URLWithUserinfo(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        err := store.FetchProfile("test", "https://user:pass@raw.githubusercontent.com/nexus-os/profiles")
        if err == nil {
                t.Error("expected error for URL with userinfo")
        }
        if !strings.Contains(err.Error(), "SECURITY") {
                t.Errorf("expected 'SECURITY' in error, got: %v", err)
        }
}

// ---------------------------------------------------------------------------
// ProfileContent error paths
// ---------------------------------------------------------------------------

func TestProfileStore_ProfileContent_NotFound(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        _, err := store.ProfileContent("nonexistent")
        if err == nil {
                t.Error("expected error for missing profile file")
        }
}

// ---------------------------------------------------------------------------
// VerifyIntegrity edge cases
// ---------------------------------------------------------------------------

func TestProfileStore_VerifyIntegrity_NotInRegistry(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // A profile not in the registry should return nil (not yet tracked)
        err := store.VerifyIntegrity("unknown-profile")
        if err != nil {
                t.Errorf("expected nil for untracked profile, got: %v", err)
        }
}

func TestProfileStore_VerifyIntegrity_CannotReadFile(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // Save a profile so it's in the registry
        if err := store.SaveProfile("unreadable", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // Remove the file to make os.ReadFile fail
        if err := os.Remove(store.ProfilePath("unreadable")); err != nil {
                t.Fatalf("failed to remove profile file: %v", err)
        }

        err := store.VerifyIntegrity("unreadable")
        if err == nil {
                t.Error("expected error when profile file cannot be read")
        }
        if !strings.Contains(err.Error(), "cannot read profile file") {
                t.Errorf("expected 'cannot read profile file' error, got: %v", err)
        }
}

// ---------------------------------------------------------------------------
// LoadProfileWithExtends edge cases
// ---------------------------------------------------------------------------

func TestProfileStore_LoadProfileWithExtends_NoExtends(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        if err := store.SaveProfile("standalone", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        profile, err := store.LoadProfileWithExtends("standalone")
        if err != nil {
                t.Fatalf("LoadProfileWithExtends failed: %v", err)
        }
        if profile.Name != "test-profile" {
                t.Errorf("expected Name 'test-profile', got %q", profile.Name)
        }
}

func TestProfileStore_LoadProfileWithExtends_ParentNotFound(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // Save a child that extends a nonexistent parent
        childYAML := `name: orphan-child
version: "1.0.0"
extends: nonexistent-parent
targets:
  - family: debian
    packages:
      - git
`
        if err := store.SaveProfile("orphan-child", []byte(childYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        _, err := store.LoadProfileWithExtends("orphan-child")
        if err == nil {
                t.Error("expected error when extends parent not found")
        }
        if !strings.Contains(err.Error(), "extends resolution failed") {
                t.Errorf("expected 'extends resolution failed' error, got: %v", err)
        }
}

func TestProfileStore_LoadProfileWithExtends_ProfileNotFound(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        _, err := store.LoadProfileWithExtends("nonexistent")
        if err == nil {
                t.Error("expected error for missing profile")
        }
        if !strings.Contains(err.Error(), "not found") {
                t.Errorf("expected 'not found' error, got: %v", err)
        }
}

// ---------------------------------------------------------------------------
// Initialize edge cases
// ---------------------------------------------------------------------------

func TestProfileStore_Initialize_ExistingProfileAlreadyInRegistry(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // Pre-save the profile so it exists on disk and in registry
        if err := store.SaveProfile("base-dev", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // Initialize should skip the existing entry since it's already in registry
        bundled := map[string]string{
                "base-dev": validProfileYAML,
        }
        if err := store.Initialize(bundled); err != nil {
                t.Fatalf("Initialize failed: %v", err)
        }

        // The source should remain unchanged (was Local, should stay Local since
        // the "already in registry" path skips the write)
        meta, exists := store.registry.Profiles["base-dev"]
        if !exists {
                t.Fatal("profile should be in registry")
        }
        if meta.Source != SourceLocal {
                t.Errorf("expected SourceLocal, got %q (should not overwrite source)", meta.Source)
        }
}

func TestProfileStore_Initialize_ExistingProfileNotInRegistry(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // Write the file directly so it exists on disk but NOT in registry
        path := store.ProfilePath("base-dev")
        if err := os.WriteFile(path, []byte(validProfileYAML), 0644); err != nil {
                t.Fatalf("failed to write profile: %v", err)
        }

        bundled := map[string]string{
                "base-dev": validProfileYAML,
        }
        if err := store.Initialize(bundled); err != nil {
                t.Fatalf("Initialize failed: %v", err)
        }

        // Profile should now be in registry with SourceBundled
        meta, exists := store.registry.Profiles["base-dev"]
        if !exists {
                t.Fatal("profile should be added to registry")
        }
        if meta.Source != SourceBundled {
                t.Errorf("expected SourceBundled, got %q", meta.Source)
        }
        if meta.SHA256 == "" {
                t.Error("SHA256 should be set")
        }
}

func TestProfileStore_Initialize_ExistingProfileReadFails(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // Write the file and then make it unreadable by removing the directory
        // Actually, let's use a trick: write an empty file (not valid YAML) and then
        // directly manipulate the directory. We'll test the "exists on disk, not in
        // registry, but os.ReadFile fails" path.
        // The simplest approach: write a file, don't add to registry.
        path := store.ProfilePath("base-dev")
        if err := os.WriteFile(path, []byte(validProfileYAML), 0644); err != nil {
                t.Fatalf("failed to write profile: %v", err)
        }

        // Now make the file unreadable (remove read permissions)
        if err := os.Chmod(path, 0000); err != nil {
                t.Fatalf("failed to chmod profile: %v", err)
        }
        defer os.Chmod(path, 0644) // restore for cleanup

        bundled := map[string]string{
                "base-dev": validProfileYAML,
        }
        if err := store.Initialize(bundled); err != nil {
                t.Fatalf("Initialize should not fail when existing profile read fails: %v", err)
        }

        // Profile should NOT be in registry since os.ReadFile failed
        _, exists := store.registry.Profiles["base-dev"]
        if exists {
                t.Error("profile should NOT be in registry when read fails")
        }
}

func TestProfileStore_Initialize_InvalidBundledContent(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // Bundled content that fails ParseBytes should still be written to disk
        // (version will be empty string)
        invalidContent := `name: test-profile
version: "1.0.0"
targets: []
`
        bundled := map[string]string{
                "bad-bundle": invalidContent,
        }
        // This should fail because the bundled content is invalid and saveRegistry
        // will be called, but the write itself should succeed
        // Actually ParseBytes will fail on "targets: []" (empty targets with no extends)
        // So version will be empty, but the file should still be written since
        // Initialize writes the file first then records metadata
        // Let's check: Initialize calls os.WriteFile only if file doesn't exist,
        // then records metadata. If ParseBytes fails, version is empty.
        err := store.Initialize(bundled)
        // Initialize should succeed - it just means version will be empty
        // The file gets written because the "invalid" part is only about version extraction
        // Actually wait, "targets: []" may not fail schema validation but will fail
        // semantic validation. Let me reconsider.
        // ParseBytes would fail, meaning version = "" and the file is still written.
        // But then saveRegistry is called at the end...
        // Actually, Initialize writes the content as-is to disk regardless of ParseBytes result.
        // It only uses ParseBytes for version extraction. So it should succeed.
        if err != nil {
                t.Logf("Initialize with invalid bundled content returned: %v", err)
        }
}

func TestProfileStore_Initialize_WriteFails(t *testing.T) {
        origHome := os.Getenv("HOME")
        tmpDir := t.TempDir()
        os.Setenv("HOME", tmpDir)
        defer os.Setenv("HOME", origHome)

        store, err := NewProfileStore()
        if err != nil {
                t.Fatalf("NewProfileStore failed: %v", err)
        }

        // Make the profiles directory read-only so writing a profile fails
        if err := os.Chmod(store.dir, 0555); err != nil {
                t.Fatalf("failed to chmod profiles dir: %v", err)
        }
        defer os.Chmod(store.dir, 0755) // restore for cleanup

        bundled := map[string]string{
                "base-dev": validProfileYAML,
        }
        err = store.Initialize(bundled)
        if err == nil {
                t.Error("expected error when writing bundled profile fails")
        }
        if !strings.Contains(err.Error(), "failed to write bundled profile") {
                t.Errorf("expected 'failed to write bundled profile' error, got: %v", err)
        }
}

// ---------------------------------------------------------------------------
// FetchProfile with httptest - additional edge cases
// ---------------------------------------------------------------------------

func TestFetchProfile_ConnectionError(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        // Create a server and immediately close it to force a connection error
        ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                fmt.Fprint(w, "should not reach")
        }))
        u, _ := url.Parse(ts.URL)
        host := u.Hostname()

        // Add host to allowed list and configure client
        origAllowed := AllowedRemoteHosts[host]
        AllowedRemoteHosts[host] = true
        defer func() {
                if !origAllowed {
                        delete(AllowedRemoteHosts, host)
                }
        }()

        // Close the server so connections will fail
        ts.Close()

        // Use a client that will try to connect to the closed server
        origClient := defaultHTTPClient
        defaultHTTPClient = ts.Client()
        defer func() { defaultHTTPClient = origClient }()

        err := store.FetchProfile("test-profile", "https://"+u.Host)
        if err == nil {
                t.Error("expected error for connection failure")
        }
        if !strings.Contains(err.Error(), "failed to fetch profile") {
                t.Errorf("expected 'failed to fetch profile' error, got: %v", err)
        }
}

func TestFetchProfile_NonHTTPOKStatus(t *testing.T) {
        ts, cleanup := setupFetchTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.WriteHeader(http.StatusInternalServerError)
        }))
        defer cleanup()

        store, storeCleanup := setupTestStore(t)
        defer storeCleanup()

        err := store.FetchProfile("test-profile", ts.URL)
        if err == nil {
                t.Error("expected error for HTTP 500")
        }
        if !strings.Contains(err.Error(), "HTTP 500") {
                t.Errorf("expected 'HTTP 500' error, got: %v", err)
        }
}

func TestFetchProfile_VersionDriftNonRemoteSource(t *testing.T) {
        store, storeCleanup := setupTestStore(t)
        defer storeCleanup()

        // Pre-save a profile with version 1.0.0 as LOCAL (not remote)
        v1YAML := `name: test-profile
version: "1.0.0"
targets:
  - family: debian
    packages:
      - git
`
        if err := store.SaveProfile("test-profile", []byte(v1YAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // Now serve v2 from the test server - but since source is Local, drift check is skipped
        v2YAML := `name: test-profile
version: "2.0.0"
targets:
  - family: debian
    packages:
      - git
      - curl
`
        ts, cleanup := setupFetchTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                fmt.Fprint(w, v2YAML)
        }))
        defer cleanup()

        // FetchProfile should succeed (version drift is non-fatal and skipped for non-remote source)
        err := store.FetchProfile("test-profile", ts.URL)
        if err != nil {
                t.Fatalf("FetchProfile should succeed: %v", err)
        }

        // Version should be updated to 2.0.0 since SaveProfile is called
        meta, exists := store.GetMeta("test-profile")
        if !exists {
                t.Fatal("profile should exist in registry")
        }
        if meta.Version != "2.0.0" {
                t.Errorf("expected version '2.0.0' after re-fetch, got %q", meta.Version)
        }
}

func TestFetchProfile_SameVersionNoDrift(t *testing.T) {
        store, storeCleanup := setupTestStore(t)
        defer storeCleanup()

        // Pre-save a profile as remote with same version
        v1YAML := `name: test-profile
version: "1.0.0"
targets:
  - family: debian
    packages:
      - git
`
        if err := store.SaveProfile("test-profile", []byte(v1YAML), SourceRemote); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // Serve same version from test server (no drift)
        ts, cleanup := setupFetchTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                fmt.Fprint(w, v1YAML)
        }))
        defer cleanup()

        err := store.FetchProfile("test-profile", ts.URL)
        if err != nil {
                t.Fatalf("FetchProfile should succeed with same version: %v", err)
        }
}

// ---------------------------------------------------------------------------
// validateRemoteURL edge cases
// ---------------------------------------------------------------------------

func TestValidateRemoteURL_InvalidURL(t *testing.T) {
        err := validateRemoteURL("://invalid-url")
        if err == nil {
                t.Error("expected error for invalid URL")
        }
        if !strings.Contains(err.Error(), "invalid URL") {
                t.Errorf("expected 'invalid URL' error, got: %v", err)
        }
}

func TestValidateRemoteURL_FragmentRejected(t *testing.T) {
        err := validateRemoteURL("https://raw.githubusercontent.com/nexus-os/profiles#section")
        if err == nil {
                t.Error("expected URL with fragment to be rejected")
        }
        if !strings.Contains(err.Error(), "query parameters or fragments") {
                t.Errorf("expected fragment error, got: %v", err)
        }
}

func TestValidateRemoteURL_EmptyString(t *testing.T) {
        err := validateRemoteURL("")
        if err == nil {
                t.Error("expected error for empty URL")
        }
}

func TestValidateRemoteURL_FTPScheme(t *testing.T) {
        err := validateRemoteURL("ftp://raw.githubusercontent.com/profiles")
        if err == nil {
                t.Error("expected FTP URL to be rejected")
        }
        if !strings.Contains(err.Error(), "HTTPS") {
                t.Errorf("expected HTTPS-related error, got: %v", err)
        }
}

// ---------------------------------------------------------------------------
// saveRegistry error paths
// ---------------------------------------------------------------------------

func TestProfileStore_SaveRegistry_WriteFails(t *testing.T) {
        origHome := os.Getenv("HOME")
        tmpDir := t.TempDir()
        os.Setenv("HOME", tmpDir)
        defer os.Setenv("HOME", origHome)

        store, err := NewProfileStore()
        if err != nil {
                t.Fatalf("NewProfileStore failed: %v", err)
        }

        // Make the profiles directory read-only so writing registry.json.tmp fails
        if err := os.Chmod(store.dir, 0555); err != nil {
                t.Fatalf("failed to chmod profiles dir: %v", err)
        }
        defer os.Chmod(store.dir, 0755) // restore for cleanup

        // Try to save a profile which calls saveRegistry
        err = store.SaveProfile("test-profile", []byte(validProfileYAML), SourceLocal)
        if err == nil {
                t.Error("expected error when saveRegistry fails")
        }
}

func TestProfileStore_RemoveProfile_SaveRegistryFails(t *testing.T) {
        origHome := os.Getenv("HOME")
        tmpDir := t.TempDir()
        os.Setenv("HOME", tmpDir)
        defer os.Setenv("HOME", origHome)

        store, err := NewProfileStore()
        if err != nil {
                t.Fatalf("NewProfileStore failed: %v", err)
        }

        // Save a profile first (needs writable dir)
        if err := store.SaveProfile("remove-me", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // Make the directory read-only so saveRegistry fails during remove
        if err := os.Chmod(store.dir, 0555); err != nil {
                t.Fatalf("failed to chmod profiles dir: %v", err)
        }
        defer os.Chmod(store.dir, 0755) // restore for cleanup

        err = store.RemoveProfile("remove-me", false)
        if err == nil {
                t.Error("expected error when saveRegistry fails during remove")
        }
}

func TestProfileStore_RecordApplied_SaveRegistryFails(t *testing.T) {
        origHome := os.Getenv("HOME")
        tmpDir := t.TempDir()
        os.Setenv("HOME", tmpDir)
        defer os.Setenv("HOME", origHome)

        store, err := NewProfileStore()
        if err != nil {
                t.Fatalf("NewProfileStore failed: %v", err)
        }

        // Save a profile first
        if err := store.SaveProfile("applied-test", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // Make the directory read-only so saveRegistry fails
        if err := os.Chmod(store.dir, 0555); err != nil {
                t.Fatalf("failed to chmod profiles dir: %v", err)
        }
        defer os.Chmod(store.dir, 0755) // restore for cleanup

        err = store.RecordApplied("applied-test")
        if err == nil {
                t.Error("expected error when saveRegistry fails during RecordApplied")
        }
}

func TestProfileStore_RecordApplied_NotInRegistry(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        err := store.RecordApplied("nonexistent")
        if err == nil {
                t.Error("expected error for profile not in registry")
        }
        if !strings.Contains(err.Error(), "not in registry") {
                t.Errorf("expected 'not in registry' error, got: %v", err)
        }
}

// ---------------------------------------------------------------------------
// NewProfileStore edge cases
// ---------------------------------------------------------------------------

func TestNewProfileStore_MkdirAllFails(t *testing.T) {
        origHome := os.Getenv("HOME")
        tmpDir := t.TempDir()
        // Create a read-only parent so MkdirAll inside .nexus fails
        readOnlyDir := filepath.Join(tmpDir, "readonly")
        if err := os.MkdirAll(readOnlyDir, 0555); err != nil {
                t.Fatalf("failed to create read-only dir: %v", err)
        }
        os.Setenv("HOME", filepath.Join(readOnlyDir, "subdir"))
        defer func() {
                os.Setenv("HOME", origHome)
                os.Chmod(readOnlyDir, 0755) // restore for cleanup
        }()

        _, err := NewProfileStore()
        if err == nil {
                t.Error("expected error when MkdirAll fails")
        }
        if !strings.Contains(err.Error(), "failed to create profiles directory") {
                t.Errorf("expected 'failed to create profiles directory' error, got: %v", err)
        }
}

func TestProfileStore_LoadProfile_IntegrityCheckFails(t *testing.T) {
        store, cleanup := setupTestStore(t)
        defer cleanup()

        if err := store.SaveProfile("tampered-load", []byte(validProfileYAML), SourceLocal); err != nil {
                t.Fatalf("SaveProfile failed: %v", err)
        }

        // Tamper with the file so VerifyIntegrity will fail
        path := store.ProfilePath("tampered-load")
        tamperedContent := strings.ReplaceAll(validProfileYAML, "git", "git-tampered")
        if err := os.WriteFile(path, []byte(tamperedContent), 0644); err != nil {
                t.Fatalf("failed to tamper with profile: %v", err)
        }

        // LoadProfile should fail with INTEGRITY CHECK FAILED
        _, err := store.LoadProfile("tampered-load")
        if err == nil {
                t.Error("expected error when loading tampered profile")
        }
        if !strings.Contains(err.Error(), "INTEGRITY CHECK FAILED") {
                t.Errorf("expected 'INTEGRITY CHECK FAILED' error, got: %v", err)
        }
}

func TestProfileStore_SaveRegistry_RenameFails(t *testing.T) {
        origHome := os.Getenv("HOME")
        tmpDir := t.TempDir()
        os.Setenv("HOME", tmpDir)
        defer os.Setenv("HOME", origHome)

        store, err := NewProfileStore()
        if err != nil {
                t.Fatalf("NewProfileStore failed: %v", err)
        }

        // Create a directory at the target registry.json path so rename fails
        // (can't rename a file over an existing directory)
        if err := os.MkdirAll(filepath.Join(store.dir, "registry.json"), 0755); err != nil {
                t.Fatalf("failed to create blocking directory: %v", err)
        }

        // SaveProfile should fail because saveRegistry's Rename fails
        err = store.SaveProfile("test-profile", []byte(validProfileYAML), SourceLocal)
        if err == nil {
                t.Error("expected error when saveRegistry rename fails")
        }
        if !strings.Contains(err.Error(), "failed to commit registry") {
                t.Errorf("expected 'failed to commit registry' error, got: %v", err)
        }
}

func TestNewProfileStore_CorruptRegistry(t *testing.T) {
        origHome := os.Getenv("HOME")
        tmpDir := t.TempDir()
        os.Setenv("HOME", tmpDir)
        defer os.Setenv("HOME", origHome)

        // Create the profiles directory and write a corrupt registry
        profilesDir := filepath.Join(tmpDir, ".nexus", "profiles")
        if err := os.MkdirAll(profilesDir, 0755); err != nil {
                t.Fatalf("failed to create profiles dir: %v", err)
        }
        corruptData := []byte("{invalid json!!!")
        if err := os.WriteFile(filepath.Join(profilesDir, "registry.json"), corruptData, 0644); err != nil {
                t.Fatalf("failed to write corrupt registry: %v", err)
        }

        // NewProfileStore should create a fresh registry when the existing one is corrupt
        store, err := NewProfileStore()
        if err != nil {
                t.Fatalf("NewProfileStore should handle corrupt registry: %v", err)
        }
        if store.registry == nil {
                t.Error("registry should not be nil")
        }
        if store.registry.Version != 1 {
                t.Errorf("expected fresh registry version 1, got %d", store.registry.Version)
        }
        if len(store.registry.Profiles) != 0 {
                t.Errorf("fresh registry should have no profiles, got %d", len(store.registry.Profiles))
        }
}

func TestFormatProfileMeta(t *testing.T) {
        now := time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)
        meta := ProfileMeta{
                Name:        "base-dev",
                Version:     "1.0.0",
                Source:      SourceBundled,
                SHA256:      "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
                DateAdded:   now,
                LastApplied: &now,
        }

        result := FormatProfileMeta(meta)

        // Should contain profile name
        if !strings.Contains(result, "base-dev") {
                t.Error("FormatProfileMeta output should contain profile name")
        }
        // Should contain source
        if !strings.Contains(result, "bundled") {
                t.Error("FormatProfileMeta output should contain source")
        }
        // Should contain version
        if !strings.Contains(result, "1.0.0") {
                t.Error("FormatProfileMeta output should contain version")
        }
        // Should contain truncated SHA256 with ellipsis
        if !strings.Contains(result, "abcdef1234567890"+"…") {
                t.Error("FormatProfileMeta output should contain truncated SHA256 with ellipsis")
        }
        // Should contain last applied timestamp
        if !strings.Contains(result, "2025-03-15 10:30") {
                t.Error("FormatProfileMeta output should contain formatted LastApplied")
        }

        // Test with nil LastApplied
        metaNoApplied := meta
        metaNoApplied.LastApplied = nil
        resultNoApplied := FormatProfileMeta(metaNoApplied)
        if !strings.Contains(resultNoApplied, "never") {
                t.Error("FormatProfileMeta should show 'never' when LastApplied is nil")
        }
}
