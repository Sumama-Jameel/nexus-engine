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

package dotfiles

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// AddDeps holds dependencies for the Add / Verify operations.
// Mirrors ApplyDeps for consistency.
type AddDeps struct {
	ExecFn ExecFunc
	State  *engine.StateTracker
	Audit  *engine.AuditLogger
}

// TrackResult captures the outcome of an Add operation.
type TrackResult struct {
	Added bool   `json:"added"`
	Path  string `json:"path"`
}

// Add validates a path and registers it with chezmoi via `chezmoi add`.
//
// Validation rules (per V7 plan D7):
//   - Path must be absolute and normalized.
//   - Path must not contain '..' (no traversal).
//   - Path must not contain shell metacharacters.
//   - Path must live under the user's $HOME directory.
//   - Sensitive paths require AllowSensitive=true.
//
// On success, the path is appended to deps.State's ManagedFiles list.
// On failure, no state mutation occurs (chezmoi add is atomic).
func Add(ctx context.Context, path string, deps AddDeps, allowSensitive bool) (*TrackResult, error) {
	if deps.ExecFn == nil {
		return nil, fmt.Errorf("dotfiles: AddDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}

	if err := validateManagedPath(path, allowSensitive); err != nil {
		return nil, fmt.Errorf("invalid managed path: %w", err)
	}

	if _, err := deps.ExecFn(ctx, "chezmoi", "add", "--", path); err != nil {
		return nil, fmt.Errorf("chezmoi add failed: %w", err)
	}

	if deps.State != nil {
		_ = deps.State.RecordDotfilesAdd(path)
	}

	return &TrackResult{
		Added: true,
		Path:  path,
	}, nil
}

// VerifyReport captures the outcome of a Verify operation.
//
// We don't track per-file SHA256 baselines in V7 (that requires hashing
// at Add time, which we deliberately defer \u2014 see V7 plan §10). Instead,
// Verify runs `chezmoi verify` which compares the live system against
// the bound source. It exits 0 when the system matches, exit 1 when
// there are differences.
type VerifyReport struct {
	OK           bool   `json:"ok"`
	Message      string `json:"message,omitempty"`
	ManagedCount int    `json:"managed_count"`
}

// Verify checks that the live system matches the bound source by running
// `chezmoi verify`. Returns a typed report; does NOT return an error when
// differences are found (that would conflate "drift detected" with
// "execution failed").
func Verify(ctx context.Context, deps AddDeps) (*VerifyReport, error) {
	if deps.ExecFn == nil {
		return nil, fmt.Errorf("dotfiles: AddDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}

	managedCount := 0
	if deps.State != nil {
		managedCount = len(deps.State.GetDotfilesState().ManagedFiles)
	}

	out, err := deps.ExecFn(ctx, "chezmoi", "verify")
	if err == nil {
		return &VerifyReport{
			OK:           true,
			Message:      strings.TrimSpace(out),
			ManagedCount: managedCount,
		}, nil
	}

	// chezmoi verify exits 1 when differences exist. Treat that as a
	// successful Verify call that found drift, not as a hard error.
	if strings.Contains(err.Error(), "exit status 1") {
		return &VerifyReport{
			OK:           false,
			Message:      "drift detected \u2014 run 'chezmoi diff' for details",
			ManagedCount: managedCount,
		}, nil
	}

	// Any other error (timeout, command not found, etc.) is a real failure.
	return nil, fmt.Errorf("chezmoi verify failed: %w", err)
}

// validateManagedPath enforces the path safety rules for chezmoi-managed files.
//
// The validation is deliberately strict and is performed BEFORE any
// chezmoi invocation \u2014 a bad path must never reach the subprocess.
func validateManagedPath(path string, allowSensitive bool) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}

	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute (got %q)", path)
	}

	cleanPath := filepath.Clean(path)
	if cleanPath != path {
		return fmt.Errorf("path must be normalized (got %q, cleaned to %q)", path, cleanPath)
	}

	if strings.Contains(path, "..") {
		return fmt.Errorf("path must not contain '..' (traversal)")
	}

	if hasShellMetacharacters(path) {
		return fmt.Errorf("path must not contain shell metacharacters")
	}

	if !allowSensitive && isSensitivePath(path) {
		return fmt.Errorf("path %q is sensitive (SSH keys, GPG, cloud creds); pass --force to override", path)
	}

	// Containment check: path must live under $HOME.
	home, err := os.UserHomeDir()
	if err != nil {
		// If we cannot determine $HOME, refuse rather than risk a wrong
		// containment decision.
		return fmt.Errorf("cannot determine user home directory: %w", err)
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return fmt.Errorf("path %q is outside $HOME (%s)", path, home)
	}

	return nil
}

// isSensitivePath reports whether a path matches one of the well-known
// sensitive-path patterns. The list is intentionally conservative:
// false negatives (allowing a non-sensitive path) are acceptable;
// false positives (blocking a non-sensitive path) just require --force.
func isSensitivePath(path string) bool {
	// Match by substring on the FULL path, not just the basename \u2014
	// ~/.ssh/id_rsa must match `.ssh/id_` even though basename is `id_rsa`.
	sensitiveMarkers := []string{
		".ssh/id_",            // private SSH keys
		".ssh/known_hosts",    // SSH host fingerprints
		".gnupg/",             // GPG keys and config
		".gnupg-private/",     // GPG private keys (alternative location)
		".aws/credentials",    // AWS access keys
		".azure/",             // Azure CLI credentials
		".gcloud/",            // GCP credentials
		".config/gh/hosts",    // GitHub CLI hosts
		".netrc",              // legacy plaintext credentials
		".docker/config.json", // Docker registry auth
	}
	for _, marker := range sensitiveMarkers {
		if strings.Contains(path, marker) {
			return true
		}
	}
	return false
}

// hasShellMetacharacters mirrors engine.containsShellMetacharacters
// (which is unexported). We re-declare a small version here rather than
// exporting the engine helper, because the dotfiles package should be
// self-contained for its validation rules.
//
// If this list ever needs to diverge from the engine's list, that
// divergence is a security signal worth a code review.
func hasShellMetacharacters(s string) bool {
	dangerous := []string{";", "|", "&", "$", "`", "(", ")", "{", "}", "<", ">", "\n", "\r", "'", "\"", "\\", "!"}
	for _, c := range dangerous {
		if strings.Contains(s, c) {
			return true
		}
	}
	return false
}
