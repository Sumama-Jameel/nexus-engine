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
	"strings"
)

// VerifyResult captures the post-installation verification outcome for a single package.
type VerifyResult struct {
	// Package is the name of the package that was verified.
	Package string `json:"package"`
	// Verified indicates whether the package passed both the package-manager
	// installation check and the binary-level functional test.
	Verified bool `json:"verified"`
	// Status classifies the verification outcome. One of "verified",
	// "not_found" (package manager does not report it installed), or
	// "broken" (installed but binary check failed).
	Status string `json:"status"`
}

// VerifyInstallation confirms each package is actually functional after install.
// Per the Zero-Trust principle: "Trust but verify." An exit code 0 from
// apt-get does NOT guarantee the package works. Post-install scripts
// may have failed, the binary may be in a non-standard path, or the
// package database may be stale.
//
// The execFn parameter is the SanitizeAndExecute function — this is
// dependency injection, ensuring the verifier never bypasses the
// security gate and works with ALL package managers (not just apt).
func VerifyInstallation(ctx context.Context, pm PackageManager, packages []string, execFn ExecFunc) []VerifyResult {
	var results []VerifyResult

	for _, pkg := range packages {
		result := VerifyResult{Package: pkg}

		// Step 1: Check if the package manager reports it as installed
		if !pm.IsInstalled(ctx, pkg) {
			result.Verified = false
			result.Status = "not_found"
			results = append(results, result)
			continue
		}

		// Step 2: For critical packages, verify the binary is actually callable
		if verified := verifyBinary(ctx, execFn, pkg); !verified {
			result.Verified = false
			result.Status = "broken"
			results = append(results, result)
			continue
		}

		result.Verified = true
		result.Status = "verified"
		results = append(results, result)
	}

	return results
}

// verifyBinary attempts to confirm the package's main binary is functional.
// This is a best-effort check — not all packages have a binary with --version.
//
// Per DDD: this function depends on the ExecFunc abstraction, not on any
// concrete PackageManager implementation. It works identically whether the
// backend is apt, pacman, dnf, or apk.
func verifyBinary(ctx context.Context, execFn ExecFunc, pkg string) bool {
	// Map of packages to their verification commands
	// Only critical packages get binary-level verification
	verifiable := map[string][]string{
		"git":     {"git", "--version"},
		"curl":    {"curl", "--version"},
		"wget":    {"wget", "--version"},
		"vim":     {"vim", "--version"},
		"python3": {"python3", "--version"},
		"python":  {"python", "--version"},
		"nodejs":  {"node", "--version"},
		"npm":     {"npm", "--version"},
		"zsh":     {"zsh", "--version"},
		"htop":    {"htop", "--version"},
		"tmux":    {"tmux", "-V"},
		"java":    {"java", "-version"},
		"chezmoi": {"chezmoi", "--version"},
	}

	args, known := verifiable[pkg]
	if !known {
		// Unknown packages: trust the package manager's IsInstalled check
		return true
	}

	_, err := execFn(ctx, args[0], args[1:]...)
	if err != nil {
		// For Java, -version outputs to stderr but returns 0
		// SanitizeAndExecute treats stderr output as an error,
		// but the binary IS functional if the command ran at all
		if strings.Contains(pkg, "java") || strings.Contains(pkg, "jdk") {
			// If the error is just "EXEC: ... (stderr: ...)" with no real
			// failure, Java is functional. Check if it's a stderr-only "error"
			if strings.Contains(err.Error(), "stderr:") &&
				!strings.Contains(err.Error(), "not found") &&
				!strings.Contains(err.Error(), "no such file") {
				return true
			}
		}
		return false
	}

	return true
}

// FormatVerifyResults returns a human-readable summary.
func FormatVerifyResults(results []VerifyResult) string {
	verified := 0
	broken := 0
	notFound := 0

	for _, r := range results {
		switch r.Status {
		case "verified":
			verified++
		case "broken":
			broken++
		case "not_found":
			notFound++
		}
	}

	return fmt.Sprintf("  Verified: %d ✅  Broken: %d ⛔  Not Found: %d ❌", verified, broken, notFound)
}
