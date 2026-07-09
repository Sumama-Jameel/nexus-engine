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
	"strings"
	"time"

	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
)

// ExecFunc is the dependency-injected command execution function.
// It is an alias for installer.ExecFunc so callers can pass either
// engine.SanitizeAndExecute directly or any compatible function.
//
// Per Zero-Trust: the dotfiles package NEVER calls exec.Command directly.
// All subprocess execution flows through the caller-provided ExecFunc, which
// must enforce the engine's allowlist, argument sanitization, and timeout.
type ExecFunc = installer.ExecFunc

// Detect probes the host for a Chezmoi installation and returns a populated
// DetectReport.
//
// Behavior:
//   - Runs `chezmoi --version` via the injected ExecFunc.
//   - On success: parses the version, populates Installed=true and paths.
//   - On "not found" or non-zero exit: returns a report with Installed=false
//     and a nil error. "Not installed" is a valid state, not a failure.
//   - On other errors (e.g., context cancellation): returns a non-nil error.
//
// The function never panics and never returns a partial report on success.
func Detect(ctx context.Context, execFn ExecFunc) (*DetectReport, error) {
	if execFn == nil {
		return nil, fmt.Errorf("dotfiles: ExecFunc must not be nil (Zero-Trust boundary)")
	}

	report := &DetectReport{
		ProbedAt: time.Now().UTC(),
	}

	out, err := execFn(ctx, "chezmoi", "--version")
	if err != nil {
		if isNotInstalled(err) {
			report.Installed = false
			return report, nil
		}
		return nil, fmt.Errorf("dotfiles: probe failed: %w", err)
	}

	version := parseChezmoiVersion(out)
	if version == "" {
		return nil, fmt.Errorf("dotfiles: could not parse chezmoi version from output: %q", strings.TrimSpace(out))
	}

	report.Installed = true
	report.Version = version
	report.ConfigDir = chezmoiConfigDir()
	report.SourceDir = chezmoiSourceDir()

	return report, nil
}

// parseChezmoiVersion extracts the semver X.Y.Z from `chezmoi --version` output.
//
// Real-world output examples (verified against chezmoi 2.x):
//   "chezmoi version 2.50.0\n"                                            (older builds)
//   "chezmoi version v2.50.0, commit abc123, built ..." (newer builds with 'v' prefix)
//   "chezmoi version 2.50.0\ncommit: abc123\nbuilt: 2024-01-01\n"
//   "chezmoi version 2.50.0-1-gabc123-dirty\n"  (dev builds)
//
// We return the FIRST token that looks like a clean semver after stripping
// any leading 'v', any pre-release/build suffix (anything after '-' or '+'),
// and any trailing punctuation (commas, etc.).
func parseChezmoiVersion(output string) string {
	for _, f := range strings.Fields(output) {
		// Strip a leading 'v' (e.g., "v2.50.0").
		f = strings.TrimPrefix(f, "v")
		// Strip pre-release/build suffix (e.g., "2.50.0-rc1" → "2.50.0").
		if i := strings.IndexAny(f, "-+"); i >= 0 {
			f = f[:i]
		}
		// Strip trailing punctuation (e.g., "2.50.0," → "2.50.0").
		f = strings.TrimRight(f, ",.;:!?")
		if isSemver(f) {
			return f
		}
	}
	return ""
}

// isSemver reports whether s is a strict X.Y.Z numeric version.
// We intentionally reject "v"-prefixed, pre-release, and build metadata
// strings here — those are stripped by parseChezmoiVersion before this check.
func isSemver(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// isNotInstalled inspects an error from ExecFunc (typically
// engine.SanitizeAndExecute) to determine if it means "binary not on PATH"
// versus a real execution failure.
//
// We treat both "not found" and any non-zero exit from `chezmoi --version`
// as "not installed". The rationale: a healthy chezmoi never exits non-zero
// on `--version`, so a non-zero exit indicates a broken or missing binary.
// The user-facing action in either case is the same: run `nexus dotfiles install`.
//
// Trade-off: this is string-based matching against wrapped subprocess errors.
// A future refactor could introduce typed errors in engine.SanitizeAndExecute
// (e.g., ErrBinaryNotFound), but that is out of scope for V7.
func isNotInstalled(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	notInstalledMarkers := []string{
		"executable file not found",
		"not found in",
		"exit status", // any non-zero exit from chezmoi --version
	}
	for _, marker := range notInstalledMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
