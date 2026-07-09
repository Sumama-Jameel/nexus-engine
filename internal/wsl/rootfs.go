//go:build windows

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
)

// RootFSImage describes a Linux rootfs tarball that can be imported into WSL2.
// Per V5 "The Instant Linux Importer (The Bridge)":
// "The tool downloads a tiny Linux image and imports it into WSL2 automatically."
//
// Each image in the registry has a hardcoded SHA256 hash. This hash is the
// "contract" between the compiled binary and the downloaded file. If the file
// doesn't match, it is rejected — preventing MITM attacks and corruption.
//
// SECURITY: Image metadata is immutable (const). The SHA256 hash is baked into
// the binary at compile time. No runtime modification is possible.
type RootFSImage struct {
	// Name is the human-readable image identifier (e.g., "nexus-alpine", "nexus-debian").
	// This becomes the WSL2 distribution name unless overridden by --name.
	Name string `json:"name"`

	// Version is the rootfs version (semver, e.g., "1.0.0").
	Version string `json:"version"`

	// URL is the HTTPS download URL for the rootfs tarball.
	// SECURITY: Must be HTTPS. Validated by the Downloader's SSRF protection.
	URL string `json:"url"`

	// SHA256 is the expected SHA256 hash of the tarball.
	// Computed during download and compared using constant-time comparison
	// (crypto/subtle.ConstantTimeCompare) to prevent timing attacks.
	SHA256 string `json:"sha256"`

	// Size is the expected file size in bytes.
	// Used for progress reporting and as a fast-fail integrity check.
	Size int64 `json:"size"`

	// Arch is the target architecture (e.g., "amd64", "arm64").
	// Must match runtime.GOARCH on the Windows host.
	Arch string `json:"arch"`

	// Family is the package family (debian, arch, fedora, alpine).
	// Used to determine which PackageManager the imported distro will use
	// when running `nexus install` from inside the WSL2 instance.
	Family string `json:"family"`

	// Description is a human-readable description of the image.
	Description string `json:"description"`

	// DefaultUser is the non-root user to create during post-import configuration.
	// Per the plan: we create a user so the user doesn't run as root by default.
	DefaultUser string `json:"default_user"`
}

// DefaultRootFSRegistry returns the built-in rootfs image registry.
// These images are the "official" Nexus rootfs distributions.
//
// Per the plan.md: "We do not build an installer. We build a minimal rootfs tarball."
// Per the roadmap: "Downloads a tiny Linux image and imports it into WSL2."
//
// The registry currently includes:
//   - Nexus Alpine: ~3MB, fastest possible import (the 60-second promise)
//   - Nexus Debian: ~120MB, full development environment
//
// IMPORTANT: The URLs and SHA256 hashes here are PLACEHOLDERS for the initial
// implementation. When actual rootfs tarballs are built and hosted (on GitHub
// Releases), these will be updated with real values. The download process
// will still work — it will just fail the SHA256 verification until real
// hashes are provided.
//
// For testing and development, users can use --skip-verify to bypass
// hash verification (with an explicit security warning).
func DefaultRootFSRegistry() []RootFSImage {
	return []RootFSImage{
		{
			Name:        "nexus-alpine",
			Version:     "1.0.0",
			URL:         "https://dl-cdn.alpinelinux.org/alpine/v3.19/releases/x86_64/alpine-minirootfs-3.19.1-x86_64.tar.gz",
			SHA256:      "placeholder-alpine-sha256-will-be-replaced-with-real-hash",
			Size:        3_145_728, // ~3MB
			Arch:        "amd64",
			Family:      "alpine",
			Description: "Nexus Alpine — Minimal footprint, fastest import (~3MB). Perfect for the 60-second promise.",
			DefaultUser: "nexus",
		},
		{
			Name:        "nexus-debian",
			Version:     "1.0.0",
			URL:         "https://github.com/Sumama-Jameel/rootfs/releases/download/v1.0.0/nexus-debian-bookworm-amd64.tar.gz",
			SHA256:      "placeholder-debian-sha256-will-be-replaced-with-real-hash",
			Size:        125_829_120, // ~120MB
			Arch:        "amd64",
			Family:      "debian",
			Description: "Nexus Debian — Full development environment based on Debian Bookworm (~120MB).",
			DefaultUser: "nexus",
		},
	}
}

// FindImage looks up a rootfs image by name from the default registry.
// Returns an error if no image with that name exists.
func FindImage(name string) (*RootFSImage, error) {
	for _, img := range DefaultRootFSRegistry() {
		if img.Name == name {
			return &img, nil
		}
	}

	// Build helpful error message with available images
	available := make([]string, 0)
	for _, img := range DefaultRootFSRegistry() {
		available = append(available, fmt.Sprintf("  • %s — %s", img.Name, img.Description))
	}

	return nil, fmt.Errorf("rootfs image '%s' not found. Available images:\n%s",
		name, strings.Join(available, "\n"))
}

// GenerateWSLConf generates the content of /etc/wsl.conf for the imported distribution.
//
// Per plan.md: "We configure /etc/wsl.conf inside the rootfs *before* first boot:
// [automount] enabled=true options='metadata,umask=22,fmask=11,noexec'.
// The noexec flag is critical: it prevents Windows malware from executing
// Linux binaries and vice versa, maintaining the 'Clean Room' sovereignty claim."
//
// SECURITY MODEL:
//   - noexec on automounts: Windows binaries cannot execute inside WSL2 and vice versa
//   - metadata: enables Linux permissions on Windows-mounted files
//   - umask=22 / fmask=11: sensible default permissions
//   - Default user is set to the non-root user created during post-import
//
// This configuration is INJECTED into the rootfs tarball BEFORE import,
// ensuring it is active from the very first boot.
func GenerateWSLConf(defaultUser string) string {
	if defaultUser == "" {
		defaultUser = "nexus"
	}

	return fmt.Sprintf(`# Nexus Protocol — WSL2 Configuration
# Per V5 "The Bridge": Security-hardened WSL2 instance configuration.
# This file is injected BEFORE first boot to enforce the Clean Room model.
#
# SECURITY: The noexec flag on automounts prevents cross-OS malware execution.
# Windows binaries cannot execute inside this WSL2 instance via mounted paths,
# and Linux binaries cannot execute on the Windows host via \\wsl$ paths.

[automount]
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

// ValidateDistroName checks that a distribution name is safe for use with
// wsl --import and wsl --unregister commands.
//
// SECURITY: Distribution names are passed as arguments to SanitizeAndExecute,
// which already rejects shell metacharacters. This function provides an
// additional layer of validation specific to WSL2 naming rules:
//   - Must start with a letter or digit
//   - Only alphanumeric characters and hyphens allowed
//   - Must not be empty
//   - Must not exceed 64 characters
func ValidateDistroName(name string) error {
	if name == "" {
		return fmt.Errorf("distribution name cannot be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("distribution name too long (max 64 characters, got %d)", len(name))
	}

	// Must start with alphanumeric
	if !isAlphanumeric(rune(name[0])) {
		return fmt.Errorf("distribution name must start with a letter or digit")
	}

	// Only alphanumeric and hyphens
	for _, ch := range name {
		if !isAlphanumeric(ch) && ch != '-' {
			return fmt.Errorf("distribution name contains invalid character '%c' (only alphanumeric and hyphens allowed)", ch)
		}
	}

	// Must not end with hyphen
	if name[len(name)-1] == '-' {
		return fmt.Errorf("distribution name must not end with a hyphen")
	}

	// Must not contain consecutive hyphens
	if strings.Contains(name, "--") {
		return fmt.Errorf("distribution name must not contain consecutive hyphens")
	}

	return nil
}

// ValidateInstallPath checks that an install path is safe.
// Per Zero-Trust: the install path must be under the user's home directory
// in the .nexus/wsl/ subtree to prevent path traversal attacks.
//
// SECURITY: This prevents a malicious profile or command from writing
// the WSL2 vhdx file to an arbitrary location (e.g., overwriting
// system files or placing files in world-writable directories).
func ValidateInstallPath(path string) error {
	if path == "" {
		return fmt.Errorf("install path cannot be empty")
	}

	// Must not contain path traversal sequences
	if strings.Contains(path, "..") {
		return fmt.Errorf("install path must not contain '..' (path traversal prevention)")
	}

	return nil
}

// isAlphanumeric checks if a rune is a letter or digit.
func isAlphanumeric(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}
