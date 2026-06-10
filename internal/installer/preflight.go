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

package installer

import (
        "context"
        "fmt"
)

// PreFlightResult captures the outcome of all pre-installation checks.
type PreFlightResult struct {
        // Skipped lists packages that are already installed and will not be
        // reinstalled (idempotency).
        Skipped    []string `json:"skipped_already_installed"`
        // ToInstall lists packages that are not yet present and require installation.
        ToInstall  []string `json:"to_install"`
        // Warnings contains human-readable messages for any checks that failed
        // or produced advisories.
        Warnings   []string `json:"warnings"`
        // CanProceed indicates whether the installation may proceed. When false,
        // at least one critical check (disk, sudo, or lock) has failed.
        CanProceed bool     `json:"can_proceed"`
        // DiskOK indicates whether the filesystem has at least 500 MB of free space.
        DiskOK     bool     `json:"disk_ok"`
        // NetworkOK indicates whether the package repository is reachable.
        // A false value is non-fatal; cached packages may still be installable.
        NetworkOK  bool     `json:"network_ok"`
        // SudoOK indicates whether non-interactive sudo access is available.
        // Always true on Alpine (runs as root).
        SudoOK     bool     `json:"sudo_ok"`
        // LockOK indicates whether the package manager's lock file is not held
        // by another process.
        LockOK     bool     `json:"lock_ok"`
}

// PreFlightCheck validates the environment BEFORE any installation begins.
// Per the engineering principle: "Measure twice, cut once."
// If any critical check fails, we abort — never attempt a write
// operation on a system that can't accept it.
func PreFlightCheck(ctx context.Context, pm PackageManager, packages []string, family string) *PreFlightResult {
        result := &PreFlightResult{
                CanProceed: true,
                Skipped:    []string{},
                ToInstall:  []string{},
                Warnings:   []string{},
        }

        // CHECK 1: Disk Space — minimum 500MB free required
        result.DiskOK = checkDiskSpace(500)
        if !result.DiskOK {
                result.CanProceed = false
                result.Warnings = append(result.Warnings, "Insufficient disk space (need ≥500MB free)")
        }

        // CHECK 2: Network Connectivity — test by refreshing the package index
        // This actually tests network by attempting to reach package repositories.
        result.NetworkOK = checkNetwork(ctx, pm)
        if !result.NetworkOK {
                result.Warnings = append(result.Warnings, "No network connectivity — installation may fail")
                // Non-fatal: packages might be in cache
        }

        // CHECK 3: Sudo Access — actually test with `sudo -n true`
        // Alpine doesn't need sudo, all others do
        if family != "alpine" {
                result.SudoOK = checkSudo(ctx, pm)
                if !result.SudoOK {
                        result.CanProceed = false
                        result.Warnings = append(result.Warnings, "No sudo access — cannot install system packages")
                }
        } else {
                result.SudoOK = true // Alpine runs as root
        }

        // CHECK 4: Package Manager Lock — test by attempting to open lock files
        result.LockOK = checkLock(family)
        if !result.LockOK {
                result.CanProceed = false
                result.Warnings = append(result.Warnings, "Package manager is locked — another installation may be in progress")
        }

        // CHECK 5: Already Installed — idempotency check
        for _, pkg := range packages {
                if pm.IsInstalled(ctx, pkg) {
                        result.Skipped = append(result.Skipped, pkg)
                } else {
                        result.ToInstall = append(result.ToInstall, pkg)
                }
        }

        return result
}

// checkDiskSpace is implemented in platform-specific files:
// - preflight_linux.go  (uses syscall.Statfs)
// - preflight_windows.go (uses GetDiskFreeSpaceEx)

// checkNetwork actually tests network connectivity by attempting to refresh
// the package index. This reaches out to the configured repositories,
// proving the network path works end-to-end.
func checkNetwork(ctx context.Context, pm PackageManager) bool {
        err := pm.RefreshIndex(ctx)
        return err == nil
}

// checkSudo actually tests non-interactive sudo access by attempting
// to run `sudo -n true` through the package manager's exec function.
// The `-n` flag makes sudo non-interactive — it fails immediately
// if a password would be required.
func checkSudo(ctx context.Context, pm PackageManager) bool {
        // Try to install a no-op through the package manager.
        // The simplest test: check if we can query installed packages
        // (which typically requires read access to system databases).
        // A more direct test: try RefreshIndex which needs sudo for most PMs.
        switch v := pm.(type) {
        case *AptInstaller:
                // Test: try `sudo -n true` directly
                _, err := v.execFn(ctx, "sudo", "-n", "true")
                return err == nil
        case *PacmanInstaller:
                _, err := v.execFn(ctx, "sudo", "-n", "true")
                return err == nil
        case *DnfInstaller:
                _, err := v.execFn(ctx, "sudo", "-n", "true")
                return err == nil
        default:
                // Fallback: just check if ListInstalled works
                _, err := pm.ListInstalled(ctx)
                return err == nil
        }
}

// checkLock is implemented in platform-specific files:
// - preflight_linux.go  (uses syscall.Flock)
// - preflight_windows.go (no-op on Windows)

// FormatPreFlightResult returns a human-readable summary.
func FormatPreFlightResult(r *PreFlightResult) string {
        var output string

        if r.CanProceed {
                output += "  ✅ Pre-flight checks PASSED\n"
        } else {
                output += "  ⛔ Pre-flight checks FAILED\n"
        }

        output += fmt.Sprintf("    Disk: %s  Network: %s  Sudo: %s  Lock: %s\n",
                boolIcon(r.DiskOK), boolIcon(r.NetworkOK), boolIcon(r.SudoOK), boolIcon(r.LockOK))

        if len(r.Skipped) > 0 {
                output += fmt.Sprintf("    Skipping %d already installed: %v\n", len(r.Skipped), r.Skipped)
        }
        if len(r.ToInstall) > 0 {
                output += fmt.Sprintf("    Will install %d packages\n", len(r.ToInstall))
        }
        if len(r.Warnings) > 0 {
                for _, w := range r.Warnings {
                        output += fmt.Sprintf("    ⚠️  %s\n", w)
                }
        }

        return output
}

func boolIcon(b bool) string {
        if b {
                return "✅"
        }
        return "❌"
}
