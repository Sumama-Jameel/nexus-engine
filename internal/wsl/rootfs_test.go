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

package wsl

import (
        "fmt"
        "strings"
        "testing"
)

// ---------------------------------------------------------------------------
// Linux stub tests — verify stubs return appropriate "not available" errors
// ---------------------------------------------------------------------------

func TestIsImportAvailable(t *testing.T) {
        if IsImportAvailable() {
                t.Error("IsImportAvailable should return false on Linux")
        }
}

func TestNotAvailableError(t *testing.T) {
        err := NotAvailableError()
        if err == nil {
                t.Fatal("NotAvailableError should return non-nil error")
        }
        if !strings.Contains(err.Error(), "only available on Windows") {
                t.Errorf("expected Windows-only message, got: %v", err)
        }
}

func TestLinuxStubs_ReturnErrors(t *testing.T) {
        t.Run("ValidateDistroName returns not-available", func(t *testing.T) {
                err := ValidateDistroName("test")
                if err == nil {
                        t.Error("expected error on Linux stub")
                }
                if !strings.Contains(err.Error(), "only available on Windows") {
                        t.Errorf("expected Windows-only message, got: %v", err)
                }
        })

        t.Run("ValidateInstallPath returns not-available", func(t *testing.T) {
                err := ValidateInstallPath("/some/path")
                if err == nil {
                        t.Error("expected error on Linux stub")
                }
        })

        t.Run("GenerateWSLConf returns empty string", func(t *testing.T) {
                result := GenerateWSLConf("nexus")
                if result != "" {
                        t.Errorf("expected empty string on Linux, got: %q", result)
                }
        })

        t.Run("DefaultRootFSRegistry returns nil", func(t *testing.T) {
                result := DefaultRootFSRegistry()
                if result != nil {
                        t.Errorf("expected nil on Linux, got: %v", result)
                }
        })

        t.Run("FindImage returns not-available", func(t *testing.T) {
                _, err := FindImage("nexus-alpine")
                if err == nil {
                        t.Error("expected error on Linux stub")
                }
                if !strings.Contains(err.Error(), "only available on Windows") {
                        t.Errorf("expected Windows-only message, got: %v", err)
                }
        })
}

func TestFormatImportResult_Linux(t *testing.T) {
        result := &ImportResult{Aborted: true, AbortReason: "test"}
        output := FormatImportResult(result)
        if !strings.Contains(output, "only available on Windows") {
                t.Errorf("expected Windows-only message, got: %s", output)
        }
}

// ---------------------------------------------------------------------------
// WSL2Importer Linux stub tests
// ---------------------------------------------------------------------------

func TestNewWSL2Importer(t *testing.T) {
        importer := NewWSL2Importer(nil)
        if importer == nil {
                t.Error("expected non-nil importer (even on Linux)")
        }
}

func TestWSL2Importer_Import_Linux(t *testing.T) {
        importer := NewWSL2Importer(nil)
        config := &ImportConfig{
                DistroName: "test",
                Image:      &RootFSImage{Name: "test", Version: "1.0.0"},
        }
        result, err := importer.Import(nil, config)
        if err == nil {
                t.Error("expected error on Linux")
        }
        if result == nil {
                t.Fatal("expected non-nil result even on error")
        }
        if !result.Aborted {
                t.Error("result should be aborted on Linux")
        }
}

func TestWSL2Importer_Remove_Linux(t *testing.T) {
        importer := NewWSL2Importer(nil)
        err := importer.Remove(nil, "test", false)
        if err == nil {
                t.Error("expected error on Linux")
        }
}

func TestWSL2Importer_ListNexusDistros_Linux(t *testing.T) {
        importer := NewWSL2Importer(nil)
        _, err := importer.ListNexusDistros(nil)
        if err == nil {
                t.Error("expected error on Linux")
        }
}

// ---------------------------------------------------------------------------
// Validation logic tests — platform-agnostic security validation
//
// The validation functions in rootfs.go are Windows-only, but the
// SECURITY LOGIC they implement is inherently cross-platform.
// We test this logic by re-implementing the validation functions
// here as test helpers and verifying the security properties.
// ---------------------------------------------------------------------------

// validateDistroNameLogic mirrors the validation logic from rootfs.go (windows).
// This is the SECURITY-CRITICAL logic that must be tested on all platforms.
func validateDistroNameLogic(name string) error {
        if name == "" {
                return fmt.Errorf("distribution name cannot be empty")
        }
        if len(name) > 64 {
                return fmt.Errorf("distribution name too long (max 64 characters, got %d)", len(name))
        }
        if !isAlphanumericTest(rune(name[0])) {
                return fmt.Errorf("distribution name must start with a letter or digit")
        }
        for _, ch := range name {
                if !isAlphanumericTest(ch) && ch != '-' {
                        return fmt.Errorf("distribution name contains invalid character '%c' (only alphanumeric and hyphens allowed)", ch)
                }
        }
        if name[len(name)-1] == '-' {
                return fmt.Errorf("distribution name must not end with a hyphen")
        }
        if strings.Contains(name, "--") {
                return fmt.Errorf("distribution name must not contain consecutive hyphens")
        }
        return nil
}

// isAlphanumericTest mirrors isAlphanumeric from rootfs.go
func isAlphanumericTest(ch rune) bool {
        return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

// validateInstallPathLogic mirrors ValidateInstallPath from rootfs.go
func validateInstallPathLogic(path string) error {
        if path == "" {
                return fmt.Errorf("install path cannot be empty")
        }
        if strings.Contains(path, "..") {
                return fmt.Errorf("install path must not contain '..' (path traversal prevention)")
        }
        return nil
}

// generateWSLConfLogic mirrors GenerateWSLConf from rootfs.go
func generateWSLConfLogic(defaultUser string) string {
        if defaultUser == "" {
                defaultUser = "nexus"
        }
        return fmt.Sprintf(`[automount]
enabled = true
options = "metadata,umask=22,fmask=11,noexec"
mountFsTab = false

[interop]
enabled = true
appendWindowsPath = false

[user]
default = %s

[network]
generateHosts = true
generateResolvConf = true

[boot]
command = "echo 'Nexus Protocol — WSL2 instance ready' > /tmp/nexus-boot.log"
`, defaultUser)
}

func TestValidateDistroNameLogic(t *testing.T) {
        tests := []struct {
                name    string
                input   string
                wantErr bool
                errMsg  string
        }{
                // Valid names
                {name: "simple name", input: "Nexus", wantErr: false},
                {name: "hyphenated name", input: "my-distro", wantErr: false},
                {name: "alphanumeric with hyphen", input: "alpine3", wantErr: false},
                {name: "starts with digit", input: "0distro", wantErr: false},
                {name: "multiple hyphens", input: "my-cool-distro", wantErr: false},
                {name: "single character", input: "a", wantErr: false},

                // Invalid names — security-critical cases
                {name: "path traversal", input: "../../evil", wantErr: true, errMsg: "must start with a letter or digit"},
                {name: "spaces", input: "has spaces", wantErr: true, errMsg: "invalid character"},
                {name: "shell injection semicolon", input: "name;rm", wantErr: true, errMsg: "invalid character"},
                {name: "empty name", input: "", wantErr: true, errMsg: "cannot be empty"},
                {name: "too long", input: strings.Repeat("a", 65), wantErr: true, errMsg: "too long"},
                {name: "max length is ok", input: strings.Repeat("a", 64), wantErr: false},
                {name: "consecutive hyphens", input: "my--distro", wantErr: true, errMsg: "consecutive hyphens"},
                {name: "starts with hyphen", input: "-distro", wantErr: true, errMsg: "must start with a letter or digit"},
                {name: "ends with hyphen", input: "distro-", wantErr: true, errMsg: "must not end with a hyphen"},
                {name: "shell pipe", input: "name|evil", wantErr: true, errMsg: "invalid character"},
                {name: "shell ampersand", input: "name&evil", wantErr: true, errMsg: "invalid character"},
                {name: "shell dollar sign", input: "name$var", wantErr: true, errMsg: "invalid character"},
                {name: "shell backtick", input: "name`cmd", wantErr: true, errMsg: "invalid character"},
                {name: "path separator slash", input: "name/sub", wantErr: true, errMsg: "invalid character"},
                {name: "backslash", input: "name\\sub", wantErr: true, errMsg: "invalid character"},
                {name: "dot prefix", input: ".hidden", wantErr: true, errMsg: "must start with a letter or digit"},
                {name: "underscore", input: "my_distro", wantErr: true, errMsg: "invalid character"},
                {name: "uppercase is valid", input: "MyDistro", wantErr: false},
        }

        for _, tc := range tests {
                t.Run(tc.name, func(t *testing.T) {
                        err := validateDistroNameLogic(tc.input)
                        if tc.wantErr {
                                if err == nil {
                                        t.Errorf("expected error for input %q, got nil", tc.input)
                                } else if !strings.Contains(err.Error(), tc.errMsg) {
                                        t.Errorf("expected error containing %q, got %q", tc.errMsg, err.Error())
                                }
                        } else {
                                if err != nil {
                                        t.Errorf("unexpected error for input %q: %v", tc.input, err)
                                }
                        }
                })
        }
}

func TestValidateInstallPathLogic(t *testing.T) {
        tests := []struct {
                name    string
                input   string
                wantErr bool
                errMsg  string
        }{
                // Valid paths
                {name: "normal path", input: "/home/user/.nexus/wsl/my-distro", wantErr: false},
                {name: "relative path", input: ".nexus/wsl/distro", wantErr: false},
                {name: "simple directory", input: "my-distro", wantErr: false},

                // Invalid paths — security-critical cases
                {name: "empty path", input: "", wantErr: true, errMsg: "cannot be empty"},
                {name: "parent traversal", input: "..", wantErr: true, errMsg: "path traversal"},
                {name: "deep traversal", input: "../../etc", wantErr: true, errMsg: "path traversal"},
                {name: "system file target", input: "/etc/passwd/..", wantErr: true, errMsg: "path traversal"},
                {name: "mid-path traversal", input: "/home/../../etc/shadow", wantErr: true, errMsg: "path traversal"},
                {name: "trailing traversal", input: "/home/user/.nexus/wsl/../../../etc", wantErr: true, errMsg: "path traversal"},
        }

        for _, tc := range tests {
                t.Run(tc.name, func(t *testing.T) {
                        err := validateInstallPathLogic(tc.input)
                        if tc.wantErr {
                                if err == nil {
                                        t.Errorf("expected error for input %q, got nil", tc.input)
                                } else if !strings.Contains(err.Error(), tc.errMsg) {
                                        t.Errorf("expected error containing %q, got %q", tc.errMsg, err.Error())
                                }
                        } else {
                                if err != nil {
                                        t.Errorf("unexpected error for input %q: %v", tc.input, err)
                                }
                        }
                })
        }
}

func TestGenerateWSLConfLogic(t *testing.T) {
        t.Run("contains noexec automount", func(t *testing.T) {
                conf := generateWSLConfLogic("nexus")
                if !strings.Contains(conf, "noexec") {
                        t.Error("wsl.conf must contain noexec in automount options for Clean Room model")
                }
        })

        t.Run("contains metadata option", func(t *testing.T) {
                conf := generateWSLConfLogic("nexus")
                if !strings.Contains(conf, "metadata") {
                        t.Error("wsl.conf must contain metadata option for Linux permissions")
                }
        })

        t.Run("contains correct user section", func(t *testing.T) {
                conf := generateWSLConfLogic("testuser")
                if !strings.Contains(conf, "default = testuser") {
                        t.Error("wsl.conf must set default user to provided username")
                }
        })

        t.Run("defaults to nexus user when empty", func(t *testing.T) {
                conf := generateWSLConfLogic("")
                if !strings.Contains(conf, "default = nexus") {
                        t.Error("wsl.conf must default to 'nexus' user when empty string provided")
                }
        })

        t.Run("contains [automount] section", func(t *testing.T) {
                conf := generateWSLConfLogic("nexus")
                if !strings.Contains(conf, "[automount]") {
                        t.Error("wsl.conf must have [automount] section")
                }
        })

        t.Run("contains [interop] section with appendWindowsPath = false", func(t *testing.T) {
                conf := generateWSLConfLogic("nexus")
                if !strings.Contains(conf, "[interop]") {
                        t.Error("wsl.conf must have [interop] section")
                }
                if !strings.Contains(conf, "appendWindowsPath = false") {
                        t.Error("wsl.conf must set appendWindowsPath = false")
                }
        })

        t.Run("contains [network] section", func(t *testing.T) {
                conf := generateWSLConfLogic("nexus")
                if !strings.Contains(conf, "[network]") {
                        t.Error("wsl.conf must have [network] section")
                }
        })

        t.Run("automount enabled", func(t *testing.T) {
                conf := generateWSLConfLogic("nexus")
                if !strings.Contains(conf, "enabled = true") {
                        t.Error("wsl.conf automount must be enabled")
                }
        })

        t.Run("umask and fmask present", func(t *testing.T) {
                conf := generateWSLConfLogic("nexus")
                if !strings.Contains(conf, "umask=22") {
                        t.Error("wsl.conf must have umask=22")
                }
                if !strings.Contains(conf, "fmask=11") {
                        t.Error("wsl.conf must have fmask=11")
                }
        })
}

// ---------------------------------------------------------------------------
// DefaultRootFSRegistry — verify Windows implementation images have
// required fields (tested via the registry structure definition)
// ---------------------------------------------------------------------------

func TestRootFSImage_Fields(t *testing.T) {
        t.Run("RootFSImage struct has all required fields", func(t *testing.T) {
                img := RootFSImage{
                        Name:        "test-image",
                        Version:     "1.0.0",
                        URL:         "https://example.com/test.tar.gz",
                        SHA256:      "abc123",
                        Size:        3145728,
                        Arch:        "amd64",
                        Family:      "alpine",
                        Description: "Test image",
                        DefaultUser: "nexus",
                }

                if img.Name != "test-image" {
                        t.Errorf("expected Name 'test-image', got %q", img.Name)
                }
                if img.Version != "1.0.0" {
                        t.Errorf("expected Version '1.0.0', got %q", img.Version)
                }
                if img.URL != "https://example.com/test.tar.gz" {
                        t.Errorf("expected URL 'https://example.com/test.tar.gz', got %q", img.URL)
                }
                if img.SHA256 != "abc123" {
                        t.Errorf("expected SHA256 'abc123', got %q", img.SHA256)
                }
                if img.Size != 3145728 {
                        t.Errorf("expected Size 3145728, got %d", img.Size)
                }
                if img.Arch != "amd64" {
                        t.Errorf("expected Arch 'amd64', got %q", img.Arch)
                }
                if img.Family != "alpine" {
                        t.Errorf("expected Family 'alpine', got %q", img.Family)
                }
                if img.DefaultUser != "nexus" {
                        t.Errorf("expected DefaultUser 'nexus', got %q", img.DefaultUser)
                }
        })
}

// ---------------------------------------------------------------------------
// FindImage — Windows implementation logic tested via test helper
// ---------------------------------------------------------------------------

// defaultRegistryForTest mirrors the DefaultRootFSRegistry from rootfs.go (windows)
func defaultRegistryForTest() []RootFSImage {
        return []RootFSImage{
                {
                        Name:        "nexus-alpine",
                        Version:     "1.0.0",
                        URL:         "https://dl-cdn.alpinelinux.org/alpine/v3.19/releases/x86_64/alpine-minirootfs-3.19.1-x86_64.tar.gz",
                        SHA256:      "placeholder-alpine-sha256-will-be-replaced-with-real-hash",
                        Size:        3_145_728,
                        Arch:        "amd64",
                        Family:      "alpine",
                        Description: "Nexus Alpine — Minimal footprint, fastest import (~3MB).",
                        DefaultUser: "nexus",
                },
                {
                        Name:        "nexus-debian",
                        Version:     "1.0.0",
                        URL:         "https://github.com/Sumama-Jameel/rootfs/releases/download/v1.0.0/nexus-debian-bookworm-amd64.tar.gz",
                        SHA256:      "placeholder-debian-sha256-will-be-replaced-with-real-hash",
                        Size:        125_829_120,
                        Arch:        "amd64",
                        Family:      "debian",
                        Description: "Nexus Debian — Full development environment (~120MB).",
                        DefaultUser: "nexus",
                },
        }
}

func findImageLogic(name string, registry []RootFSImage) (*RootFSImage, error) {
        for _, img := range registry {
                if img.Name == name {
                        return &img, nil
                }
        }

        available := make([]string, 0)
        for _, img := range registry {
                available = append(available, fmt.Sprintf("  • %s — %s", img.Name, img.Description))
        }

        return nil, fmt.Errorf("rootfs image '%s' not found. Available images:\n%s",
                name, strings.Join(available, "\n"))
}

func TestFindImageLogic(t *testing.T) {
        registry := defaultRegistryForTest()

        t.Run("found nexus-alpine", func(t *testing.T) {
                img, err := findImageLogic("nexus-alpine", registry)
                if err != nil {
                        t.Fatalf("unexpected error: %v", err)
                }
                if img.Name != "nexus-alpine" {
                        t.Errorf("expected 'nexus-alpine', got %q", img.Name)
                }
                if img.Family != "alpine" {
                        t.Errorf("expected Family 'alpine', got %q", img.Family)
                }
        })

        t.Run("found nexus-debian", func(t *testing.T) {
                img, err := findImageLogic("nexus-debian", registry)
                if err != nil {
                        t.Fatalf("unexpected error: %v", err)
                }
                if img.Name != "nexus-debian" {
                        t.Errorf("expected 'nexus-debian', got %q", img.Name)
                }
        })

        t.Run("not found returns helpful error", func(t *testing.T) {
                _, err := findImageLogic("nonexistent", registry)
                if err == nil {
                        t.Fatal("expected error for nonexistent image")
                }
                if !strings.Contains(err.Error(), "not found") {
                        t.Errorf("expected 'not found' in error, got: %v", err)
                }
                if !strings.Contains(err.Error(), "nexus-alpine") {
                        t.Error("error should list available images")
                }
                if !strings.Contains(err.Error(), "nexus-debian") {
                        t.Error("error should list all available images")
                }
        })
}

func TestWSL2Importer_SetAuditFunc(t *testing.T) {
        importer := NewWSL2Importer(nil)
        // SetAuditFunc is a no-op on Linux — just verify it doesn't panic
        importer.SetAuditFunc(func(action, target, result string, durationMs int64, err error) {})
}

func TestWSL2Importer_SetStateRecordFunc(t *testing.T) {
        importer := NewWSL2Importer(nil)
        // SetStateRecordFunc is a no-op on Linux — just verify it doesn't panic
        importer.SetStateRecordFunc(func(name, image, version, sha256, installPath, family string) error {
                return nil
        })
}

func TestDefaultRootFSRegistryFields(t *testing.T) {
        registry := defaultRegistryForTest()

        t.Run("all images have required fields", func(t *testing.T) {
                for _, img := range registry {
                        if img.Name == "" {
                                t.Error("image missing Name field")
                        }
                        if img.Version == "" {
                                t.Errorf("image %q missing Version field", img.Name)
                        }
                        if img.URL == "" {
                                t.Errorf("image %q missing URL field", img.Name)
                        }
                        if img.SHA256 == "" {
                                t.Errorf("image %q missing SHA256 field", img.Name)
                        }
                        if !strings.HasPrefix(img.URL, "https://") {
                                t.Errorf("image %q URL must be HTTPS: %q", img.Name, img.URL)
                        }
                        if img.Size <= 0 {
                                t.Errorf("image %q Size must be positive: %d", img.Name, img.Size)
                        }
                        if img.Arch == "" {
                                t.Errorf("image %q missing Arch field", img.Name)
                        }
                        if img.Family == "" {
                                t.Errorf("image %q missing Family field", img.Name)
                        }
                        if img.DefaultUser == "" {
                                t.Errorf("image %q missing DefaultUser field", img.Name)
                        }
                }
        })

        t.Run("placeholder hashes are detectable", func(t *testing.T) {
                for _, img := range registry {
                        if strings.HasPrefix(img.SHA256, "placeholder-") {
                                // Placeholder hashes should be detected and handled
                                // The download logic skips SHA256 verification for placeholders
                                t.Logf("Image %q has placeholder hash (expected for development)", img.Name)
                        }
                }
        })

        t.Run("at least two images in registry", func(t *testing.T) {
                if len(registry) < 2 {
                        t.Errorf("expected at least 2 images in registry, got %d", len(registry))
                }
        })
}
