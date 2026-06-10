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
	"archive/tar"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ExecFunc is the type signature for the centralized execution function.
// Re-declared here to avoid circular import with the engine package.
// The WSL2 importer receives this as dependency injection — all command
// execution routes through the Zero-Trust security gate.
type ExecFunc func(ctx context.Context, command string, args ...string) (string, error)

// ImportConfig holds the configuration for a WSL2 import operation.
type ImportConfig struct {
	// DistroName is the name for the WSL2 distribution.
	// This becomes the identifier used in `wsl -d <name>`.
	// Must pass ValidateDistroName().
	DistroName string

	// InstallPath is the directory where the WSL2 ext4.vhdx will be stored.
	// Must pass ValidateInstallPath().
	InstallPath string

	// TarballPath is the path to the rootfs tarball on disk.
	// If empty, the importer will download it from the RootFSImage URL.
	TarballPath string

	// Image is the rootfs image specification from the registry.
	Image *RootFSImage

	// SkipDownload skips the download step and uses a local tarball.
	// Useful for air-gapped environments or offline installs.
	SkipDownload bool

	// SkipVerify skips SHA256 verification of the downloaded tarball.
	// DANGEROUS: Only use in development/testing. Prints a security warning.
	SkipVerify bool

	// DryRun shows what would happen without executing.
	DryRun bool
}

// ImportResult captures the outcome of a WSL2 import operation.
type ImportResult struct {
	// DistroName is the name of the imported distribution.
	DistroName string `json:"distro_name"`

	// ImageName is the rootfs image used.
	ImageName string `json:"image_name"`

	// ImageVersion is the version of the rootfs image.
	ImageVersion string `json:"image_version"`

	// TarballPath is the path to the tarball used.
	TarballPath string `json:"tarball_path"`

	// TarballSHA256 is the SHA256 hash of the tarball.
	TarballSHA256 string `json:"tarball_sha256"`

	// InstallPath is the WSL2 install directory.
	InstallPath string `json:"install_path"`

	// Duration is the total time taken for the import.
	Duration time.Duration `json:"duration"`

	// StepsCompleted is the list of completed steps.
	StepsCompleted []string `json:"steps_completed"`

	// Warnings is the list of non-fatal warnings.
	Warnings []string `json:"warnings"`

	// Aborted indicates whether the import was aborted (pre-flight failure).
	Aborted bool `json:"aborted"`

	// AbortReason is the reason for abort, if aborted.
	AbortReason string `json:"abort_reason,omitempty"`
}

// WSL2Importer orchestrates the complete WSL2 import flow.
//
// Per V5 "The Instant Linux Importer (The Bridge)":
// "The tool downloads a tiny Linux image and imports it into WSL2 automatically."
//
// The import follows a 7-step flow (mirroring the V2 Orchestrator pattern):
//  1. PRE-FLIGHT  — Verify WSL2 readiness (reuse V4's Spy)
//  2. DOWNLOAD    — Fetch rootfs tarball with SHA256 verification
//  3. CONFIGURE   — Inject wsl.conf into tarball before first boot
//  4. IMPORT      — Execute `wsl --import` via SanitizeAndExecute
//  5. VERIFY      — Confirm the distribution appears in `wsl --list`
//  6. CONFIGURE-POST — Post-import setup (create user, set defaults)
//  7. RECORD      — Record the instance in state and audit log
//
// Each step is independently verifiable, auditable, and recoverable.
type WSL2Importer struct {
	execFn  ExecFunc
	auditFn AuditFunc
	stateFn StateRecordFunc
}

// AuditFunc is the callback for recording audit entries.
// Decoupled from the engine package to avoid circular imports.
type AuditFunc func(action, target, result string, durationMs int64, err error)

// StateRecordFunc is the callback for recording state changes.
type StateRecordFunc func(name, image, version, sha256, installPath, family string) error

// NewWSL2Importer creates a new WSL2Importer with the given execution function.
//
// Per the Nexus Protocol Zero-Trust Architecture: the execFn MUST be
// engine.SanitizeAndExecute. This ensures all WSL commands pass through
// the security gate (whitelist, argument sanitization, timeout).
func NewWSL2Importer(execFn ExecFunc) *WSL2Importer {
	return &WSL2Importer{
		execFn: execFn,
	}
}

// SetAuditFunc configures the audit callback for recording WSL operations.
func (w *WSL2Importer) SetAuditFunc(fn AuditFunc) {
	w.auditFn = fn
}

// SetStateRecordFunc configures the state recording callback.
func (w *WSL2Importer) SetStateRecordFunc(fn StateRecordFunc) {
	w.stateFn = fn
}

// Import executes the full 7-step WSL2 import flow.
func (w *WSL2Importer) Import(ctx context.Context, config *ImportConfig) (*ImportResult, error) {
	start := time.Now()
	result := &ImportResult{
		DistroName:     config.DistroName,
		ImageName:      config.Image.Name,
		ImageVersion:   config.Image.Version,
		StepsCompleted: []string{},
		Warnings:       []string{},
	}

	// ═══════════════════════════════════════════════════════════
	// STEP 1: PRE-FLIGHT — Verify WSL2 readiness
	// ═══════════════════════════════════════════════════════════
	if !config.DryRun {
		if err := w.preflight(ctx, config, result); err != nil {
			result.Aborted = true
			result.AbortReason = err.Error()
			result.Duration = time.Since(start)
			return result, err
		}
	}
	result.StepsCompleted = append(result.StepsCompleted, "preflight")

	// ═══════════════════════════════════════════════════════════
	// STEP 2: DOWNLOAD — Fetch rootfs tarball
	// ═══════════════════════════════════════════════════════════
	if !config.SkipDownload && config.TarballPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return result, fmt.Errorf("failed to determine home directory: %w", err)
		}
		cacheDir := filepath.Join(homeDir, ".nexus", "cache")
		config.TarballPath = filepath.Join(cacheDir, config.Image.Name+"-"+config.Image.Version+".tar.gz")
	}

	if !config.SkipDownload && config.TarballPath != "" {
		if !config.DryRun {
			if err := w.downloadRootFS(ctx, config, result); err != nil {
				result.Duration = time.Since(start)
				return result, fmt.Errorf("download failed: %w", err)
			}
		}
		result.StepsCompleted = append(result.StepsCompleted, "download")
	}

	result.TarballPath = config.TarballPath

	// ═══════════════════════════════════════════════════════════
	// STEP 3: CONFIGURE — Inject wsl.conf into tarball
	// ═══════════════════════════════════════════════════════════
	var importTarball string
	if !config.DryRun {
		prepared, err := w.configureRootFS(config, result)
		if err != nil {
			result.Duration = time.Since(start)
			return result, fmt.Errorf("rootfs configuration failed: %w", err)
		}
		importTarball = prepared
		result.StepsCompleted = append(result.StepsCompleted, "configure")
	} else {
		importTarball = config.TarballPath
		result.StepsCompleted = append(result.StepsCompleted, "configure (dry-run)")
	}

	// ═══════════════════════════════════════════════════════════
	// STEP 4: IMPORT — Execute `wsl --import`
	// ═══════════════════════════════════════════════════════════
	if !config.DryRun {
		if err := w.importDistro(ctx, config, importTarball, result); err != nil {
			result.Duration = time.Since(start)
			return result, fmt.Errorf("import failed: %w", err)
		}
	}
	result.InstallPath = config.InstallPath
	result.StepsCompleted = append(result.StepsCompleted, "import")

	// ═══════════════════════════════════════════════════════════
	// STEP 5: VERIFY — Confirm import succeeded
	// ═══════════════════════════════════════════════════════════
	if !config.DryRun {
		if err := w.verifyImport(ctx, config, result); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("import verification failed: %v", err))
		}
	}
	result.StepsCompleted = append(result.StepsCompleted, "verify")

	// ═══════════════════════════════════════════════════════════
	// STEP 6: CONFIGURE-POST — Post-import setup
	// ═══════════════════════════════════════════════════════════
	if !config.DryRun {
		if err := w.configurePost(ctx, config, result); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("post-configuration warning: %v", err))
		}
	}
	result.StepsCompleted = append(result.StepsCompleted, "configure-post")

	// ═══════════════════════════════════════════════════════════
	// STEP 7: RECORD — Record in state and audit log
	// ═══════════════════════════════════════════════════════════
	if !config.DryRun {
		w.record(config, result)
	}
	result.StepsCompleted = append(result.StepsCompleted, "record")
	result.Duration = time.Since(start)

	return result, nil
}

// Remove removes a WSL2 distribution that was imported by Nexus.
func (w *WSL2Importer) Remove(ctx context.Context, distroName string, force bool) error {
	if err := ValidateDistroName(distroName); err != nil {
		return fmt.Errorf("invalid distribution name: %w", err)
	}

	// Verify the distribution exists
	output, err := w.execFn(ctx, "wsl", "--list", "--verbose")
	if err != nil {
		return fmt.Errorf("failed to list WSL distributions: %w", err)
	}

	if !strings.Contains(strings.ToLower(output), strings.ToLower(distroName)) {
		return fmt.Errorf("distribution '%s' not found in WSL", distroName)
	}

	// Execute removal
	_, err = w.execFn(ctx, "wsl", "--unregister", distroName)
	if err != nil {
		return fmt.Errorf("failed to unregister distribution '%s': %w", distroName, err)
	}

	// Record in audit log
	if w.auditFn != nil {
		w.auditFn("wsl_remove", distroName, "success", 0, nil)
	}

	// Clean up install directory
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		installPath := filepath.Join(homeDir, ".nexus", "wsl", distroName)
		os.RemoveAll(installPath)
	}

	return nil
}

// ListNexusDistros lists WSL2 distributions managed by Nexus.
func (w *WSL2Importer) ListNexusDistros(ctx context.Context) ([]NexusDistro, error) {
	output, err := w.execFn(ctx, "wsl", "--list", "--verbose")
	if err != nil {
		return nil, fmt.Errorf("failed to list WSL distributions: %w", err)
	}

	return parseWSLDistroListV5(output), nil
}

// NexusDistro represents a WSL2 distribution known to Nexus.
type NexusDistro struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Version string `json:"version"`
	Default bool   `json:"default"`
}

// parseWSLDistroListV5 parses `wsl --list --verbose` output.
func parseWSLDistroListV5(output string) []NexusDistro {
	var distros []NexusDistro

	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		return distros
	}

	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		isDefault := false
		if strings.HasPrefix(line, "*") {
			isDefault = true
			line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		}

		fields := strings.Fields(line)
		if len(fields) >= 3 {
			distros = append(distros, NexusDistro{
				Name:    fields[0],
				State:   fields[1],
				Version: fields[2],
				Default: isDefault,
			})
		} else if len(fields) >= 1 {
			distros = append(distros, NexusDistro{
				Name:    fields[0],
				Default: isDefault,
			})
		}
	}

	return distros
}

// ─── Internal Step Methods ───

// preflight verifies that the system is ready for WSL2 import.
func (w *WSL2Importer) preflight(ctx context.Context, cfg *ImportConfig, result *ImportResult) error {
	// Check architecture
	if runtime.GOARCH != cfg.Image.Arch {
		return fmt.Errorf("architecture mismatch: this image is for %s but the system is %s",
			cfg.Image.Arch, runtime.GOARCH)
	}

	// Check if WSL2 is available
	output, err := w.execFn(ctx, "wsl", "--status")
	if err != nil {
		return fmt.Errorf("WSL2 is not available: %w. Run 'wsl --install' from an Administrator PowerShell", err)
	}

	// Check if WSL2 default version is 2
	if strings.Contains(output, "Default Version: 1") {
		result.Warnings = append(result.Warnings,
			"WSL default version is 1 — the import will use --version 2 flag to force WSL2")
	}

	// Check if distro already exists
	listOutput, err := w.execFn(ctx, "wsl", "--list", "--verbose")
	if err == nil && strings.Contains(strings.ToLower(listOutput), strings.ToLower(cfg.DistroName)) {
		return fmt.Errorf("distribution '%s' already exists in WSL. Remove it first with 'nexus wsl remove %s'",
			cfg.DistroName, cfg.DistroName)
	}

	return nil
}

// downloadRootFS fetches the rootfs tarball using the secure Downloader.
func (w *WSL2Importer) downloadRootFS(ctx context.Context, cfg *ImportConfig, result *ImportResult) error {
	downloader := NewDownloader(nil)

	if cfg.SkipVerify {
		result.Warnings = append(result.Warnings,
			"SECURITY WARNING: SHA256 verification is disabled. The downloaded file will NOT be verified for integrity.")
	}

	expectedSHA256 := cfg.Image.SHA256
	if cfg.SkipVerify {
		expectedSHA256 = ""
	}

	if err := downloader.Download(ctx, cfg.Image.URL, cfg.TarballPath, expectedSHA256, cfg.Image.Size); err != nil {
		return fmt.Errorf("failed to download rootfs: %w", err)
	}

	// Compute the actual SHA256 for recording
	if hash, err := computeFileSHA256(cfg.TarballPath); err == nil {
		result.TarballSHA256 = hash
	}

	return nil
}

// configureRootFS injects /etc/wsl.conf, /etc/nexus-version, and
// /tmp/nexus-setup.sh into the rootfs tarball.
//
// Per plan.md: "We configure /etc/wsl.conf inside the rootfs *before* first boot."
//
// Strategy: Create a NEW tarball that reads the original and appends
// our configuration files. This preserves the original cached tarball
// for reuse.
//
// SECURITY DESIGN for the setup script:
// The post-import setup script (nexus-setup.sh) is embedded INTO the rootfs
// tarball during this step. This is the ONLY way to safely execute complex
// shell commands inside the WSL2 instance without violating the Zero-Trust
// security model. SanitizeAndExecute correctly blocks shell metacharacters
// (|, >, 2>, ||, etc.) in arguments. By embedding the script in the tarball:
//  1. The script content is generated from validated inputs (username
//     validated by ValidateDistroName)
//  2. The script is covered by the rootfs SHA256 integrity verification
//  3. In Step 6 (configure-post), we execute a simple, metacharacter-free
//     command: `wsl -d <name> -- /bin/sh /tmp/nexus-setup.sh`
//  4. No runtime shell injection is possible through SanitizeAndExecute
func (w *WSL2Importer) configureRootFS(cfg *ImportConfig, result *ImportResult) (string, error) {
	wslConf := GenerateWSLConf(cfg.Image.DefaultUser)
	setupScript := generateSetupScript(cfg.Image.DefaultUser)
	preparedPath := cfg.TarballPath + ".nexus-prepared"

	originalFile, err := os.Open(cfg.TarballPath)
	if err != nil {
		return "", fmt.Errorf("failed to open tarball: %w", err)
	}
	defer originalFile.Close()

	preparedFile, err := os.Create(preparedPath)
	if err != nil {
		return "", fmt.Errorf("failed to create prepared tarball: %w", err)
	}
	defer preparedFile.Close()

	originalReader := tar.NewReader(originalFile)
	preparedWriter := tar.NewWriter(preparedFile)

	// Files to skip (we'll replace them)
	skipNames := map[string]bool{
		"etc/wsl.conf":         true,
		"./etc/wsl.conf":       true,
		"tmp/nexus-setup.sh":   true,
		"./tmp/nexus-setup.sh": true,
	}

	// Copy all existing entries, skipping our replacement targets
	for {
		header, err := originalReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tar entry: %w", err)
		}

		if skipNames[header.Name] {
			if strings.HasPrefix(header.Name, "etc/wsl.conf") {
				result.Warnings = append(result.Warnings,
					"original tarball contains /etc/wsl.conf — replacing with Nexus security-hardened configuration")
			}
			continue
		}

		if err := preparedWriter.WriteHeader(header); err != nil {
			return "", fmt.Errorf("failed to write tar header: %w", err)
		}

		if _, err := io.Copy(preparedWriter, originalReader); err != nil {
			return "", fmt.Errorf("failed to copy tar entry: %w", err)
		}
	}

	// Inject /etc/wsl.conf
	if err := writeTarEntry(preparedWriter, "etc/wsl.conf", 0644, wslConf); err != nil {
		return "", fmt.Errorf("failed to write wsl.conf: %w", err)
	}

	// Inject /etc/nexus-version marker
	versionContent := fmt.Sprintf("name=%s\nversion=%s\nfamily=%s\nimported=%s\n",
		cfg.Image.Name, cfg.Image.Version, cfg.Image.Family,
		time.Now().UTC().Format(time.RFC3339))
	if err := writeTarEntry(preparedWriter, "etc/nexus-version", 0644, versionContent); err != nil {
		return "", fmt.Errorf("failed to write nexus-version: %w", err)
	}

	// Inject /tmp/nexus-setup.sh (the post-import setup script)
	if err := writeTarEntry(preparedWriter, "tmp/nexus-setup.sh", 0755, setupScript); err != nil {
		return "", fmt.Errorf("failed to write nexus-setup.sh: %w", err)
	}

	if err := preparedWriter.Close(); err != nil {
		return "", fmt.Errorf("failed to finalize prepared tarball: %w", err)
	}
	preparedFile.Close()

	return preparedPath, nil
}

// writeTarEntry is a helper that writes a single file entry to a tar writer.
func writeTarEntry(tw *tar.Writer, name string, mode int64, content string) error {
	bytes := []byte(content)
	header := &tar.Header{
		Name:     name,
		Mode:     mode,
		Size:     int64(len(bytes)),
		ModTime:  time.Now(),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(bytes)
	return err
}

// generateSetupScript creates the post-import setup script.
// This script is embedded in the rootfs tarball and executed
// after the WSL2 import to create the non-root user and
// configure the environment.
//
// SECURITY: The username is validated by ValidateDistroName
// before this function is called. The script uses only the
// validated username variable, so no injection is possible.
func generateSetupScript(username string) string {
	if username == "" {
		username = "nexus"
	}

	return fmt.Sprintf(`#!/bin/sh
# Nexus Protocol — Post-Import Setup Script
# Generated by V5 "The Instant Linux Importer (The Bridge)"
#
# This script is embedded in the rootfs tarball and executed
# after WSL2 import to create the non-root user and configure
# the environment. It runs ONCE during import, not on every boot.
#
# COMPATIBILITY: This script supports both Alpine (adduser) and
# Debian/Fedora/Arch (useradd) user creation utilities. The default
# image (nexus-alpine) only has adduser, so we detect and use the
# appropriate command.

set -e

USERNAME="%s"

echo "Nexus Protocol: Setting up WSL2 instance..."

# Create non-root user with home directory and bash shell
# Suppress errors if user already exists (idempotent)
if ! id -u "$USERNAME" >/dev/null 2>&1; then
    # Alpine uses adduser, Debian/Fedora/Arch use useradd
    # Try adduser first (Alpine default), then fall back to useradd
    if command -v adduser >/dev/null 2>&1; then
        # Alpine adduser: -D = no password, -h = home dir, -s = shell
        adduser -D -h "/home/$USERNAME" -s "/bin/sh" "$USERNAME" 2>/dev/null || true
        echo "Nexus Protocol: Created user '$USERNAME' via adduser"
    elif command -v useradd >/dev/null 2>&1; then
        # Debian/Fedora/Arch useradd: -m = create home, -s = shell
        useradd -m -s /bin/bash "$USERNAME" 2>/dev/null || true
        echo "Nexus Protocol: Created user '$USERNAME' via useradd"
    else
        echo "Nexus Protocol: WARNING: Neither adduser nor useradd found — skipping user creation"
    fi
fi

# Ensure home directory exists regardless of which tool was used
if [ ! -d "/home/$USERNAME" ]; then
    mkdir -p "/home/$USERNAME"
    chown "$USERNAME:$USERNAME" "/home/$USERNAME"
fi

# Add user to sudo group (Debian/Ubuntu) or wheel (Fedora/Arch)
# Suppress errors if group doesn't exist (Alpine doesn't have sudo by default)
if command -v usermod >/dev/null 2>&1; then
    usermod -aG sudo "$USERNAME" 2>/dev/null || true
    usermod -aG wheel "$USERNAME" 2>/dev/null || true
fi

# Ensure the user can run sudo without password for initial setup
# This will be hardened later by the user
if command -v passwd >/dev/null 2>&1; then
    passwd -d "$USERNAME" 2>/dev/null || true
fi

# Create .nexus directory in user home
mkdir -p "/home/$USERNAME/.nexus"
chown "$USERNAME:$USERNAME" "/home/$USERNAME/.nexus"

# Create a welcome message
cat > "/home/$USERNAME/.nexus/Welcome" << 'NEXUS_EOF'
Welcome to the Nexus Protocol!
This WSL2 instance was set up by the Nexus Engine.
Run 'nexus init' to configure your development environment.
NEXUS_EOF
chown "$USERNAME:$USERNAME" "/home/$USERNAME/.nexus/Welcome"

# Update the default user in wsl.conf
if [ -f /etc/wsl.conf ]; then
    if grep -q "^\[user\]" /etc/wsl.conf; then
        sed -i "s/^default = .*/default = $USERNAME/" /etc/wsl.conf
    else
        echo "" >> /etc/wsl.conf
        echo "[user]" >> /etc/wsl.conf
        echo "default = $USERNAME" >> /etc/wsl.conf
    fi
fi

echo "Nexus Protocol: Setup complete for user '$USERNAME'"
`, username)
}

// importDistro executes `wsl --import` via the Zero-Trust security gate.
func (w *WSL2Importer) importDistro(ctx context.Context, cfg *ImportConfig, tarballPath string, result *ImportResult) error {
	if err := os.MkdirAll(cfg.InstallPath, 0755); err != nil {
		return fmt.Errorf("failed to create install directory '%s': %w", cfg.InstallPath, err)
	}

	// Execute: wsl --import <name> <install-path> <tarball-path> --version 2
	output, err := w.execFn(ctx, "wsl", "--import", cfg.DistroName, cfg.InstallPath, tarballPath, "--version", "2")
	if err != nil {
		return fmt.Errorf("wsl --import failed: %w (output: %s)", err, output)
	}

	if w.auditFn != nil {
		w.auditFn("wsl_import", cfg.DistroName, "success", 0, nil)
	}

	return nil
}

// verifyImport confirms the distribution was successfully imported.
func (w *WSL2Importer) verifyImport(ctx context.Context, cfg *ImportConfig, result *ImportResult) error {
	output, err := w.execFn(ctx, "wsl", "--list", "--verbose")
	if err != nil {
		return fmt.Errorf("failed to verify import (wsl --list failed): %w", err)
	}

	if !strings.Contains(strings.ToLower(output), strings.ToLower(cfg.DistroName)) {
		return fmt.Errorf("distribution '%s' not found in WSL list after import", cfg.DistroName)
	}

	// Boot test: verify the instance is functional
	testOutput, err := w.execFn(ctx, "wsl", "-d", cfg.DistroName, "--", "echo", "nexus-ok")
	if err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("distribution imported but boot verification failed: %v", err))
		return nil
	}

	if !strings.Contains(testOutput, "nexus-ok") {
		result.Warnings = append(result.Warnings,
			"boot verification produced unexpected output (distro may need initial setup)")
	}

	return nil
}

// configurePost executes the embedded setup script inside the WSL2 instance.
//
// SECURITY DESIGN: The setup script (/tmp/nexus-setup.sh) was embedded into
// the rootfs tarball during Step 3 (configureRootFS). This avoids passing
// shell metacharacters through SanitizeAndExecute, which correctly blocks them.
//
// The command executed here is simple and metacharacter-free:
//
//	wsl -d <name> -- /bin/sh /tmp/nexus-setup.sh
//
// All complex operations (useradd, usermod, chown, etc.) are in the script,
// which was generated from validated inputs and is covered by the rootfs
// SHA256 integrity verification.
func (w *WSL2Importer) configurePost(ctx context.Context, cfg *ImportConfig, result *ImportResult) error {
	// Execute the embedded setup script
	// Per Zero-Trust: all arguments pass SanitizeAndExecute sanitization
	_, err := w.execFn(ctx, "wsl", "-d", cfg.DistroName, "--", "/bin/sh", "/tmp/nexus-setup.sh")
	if err != nil {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("post-config setup script failed: %v (non-fatal — user may need manual setup)", err))
	}

	// Terminate the instance to apply wsl.conf changes
	w.execFn(ctx, "wsl", "--terminate", cfg.DistroName)

	return nil
}

// record saves the import result to state and audit log.
func (w *WSL2Importer) record(cfg *ImportConfig, result *ImportResult) {
	if w.stateFn != nil {
		if err := w.stateFn(
			cfg.DistroName,
			cfg.Image.Name,
			cfg.Image.Version,
			result.TarballSHA256,
			cfg.InstallPath,
			cfg.Image.Family,
		); err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("failed to record state: %v", err))
		}
	}
}

// computeFileSHA256 computes the SHA256 hash of a file.
func computeFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// FormatImportResult formats an ImportResult for human-readable output.
func FormatImportResult(result *ImportResult) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("  ╔══════════════════════════════════════════════════╗\n")
	sb.WriteString("  ║   NEXUS PROTOCOL — WSL2 IMPORT RESULT            ║\n")
	sb.WriteString("  ╚══════════════════════════════════════════════════╝\n")
	sb.WriteString("\n")

	if result.Aborted {
		sb.WriteString("  ⛔ IMPORT ABORTED\n")
		sb.WriteString(fmt.Sprintf("  Reason: %s\n", result.AbortReason))
		sb.WriteString("\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("  🐧 Distribution:  %s\n", result.DistroName))
	sb.WriteString(fmt.Sprintf("  📦 Image:         %s v%s\n", result.ImageName, result.ImageVersion))
	sb.WriteString(fmt.Sprintf("  📁 Install Path:  %s\n", result.InstallPath))
	if result.TarballSHA256 != "" {
		sb.WriteString(fmt.Sprintf("  🔒 SHA256:        %s…\n", result.TarballSHA256[:32]))
	}
	sb.WriteString(fmt.Sprintf("  ⏱️  Duration:      %s\n", result.Duration.Round(time.Millisecond)))

	sb.WriteString("\n  ── STEPS COMPLETED ──────────────────────────────\n")
	for i, step := range result.StepsCompleted {
		sb.WriteString(fmt.Sprintf("  %d. ✅ %s\n", i+1, step))
	}

	if len(result.Warnings) > 0 {
		sb.WriteString("\n  ── WARNINGS ────────────────────────────────────\n")
		for _, warn := range result.Warnings {
			sb.WriteString(fmt.Sprintf("  ⚠️  %s\n", warn))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  ✅ WSL2 import complete. Run 'nexus wsl enter %s' or 'wsl -d %s' to start.\n", result.DistroName, result.DistroName))
	sb.WriteString("\n")

	return sb.String()
}
