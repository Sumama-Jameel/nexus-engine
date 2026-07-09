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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// TeleportResult describes the outcome for one migrated folder.
type TeleportResult struct {
	// Source is the full Windows path being linked.
	Source string `json:"source"`
	// Target is the full Linux path where the symlink was created.
	Target string `json:"target"`
	// Linked is true when the symlink was created in this run.
	Linked bool `json:"linked"`
	// AlreadyExists is true when the target already existed (no action needed).
	AlreadyExists bool `json:"already_exists,omitempty"`
	// Skipped is true when the source folder didn't exist (nothing to migrate).
	Skipped bool `json:"skipped,omitempty"`
	// Error describes what went wrong, if anything.
	Error string `json:"error,omitempty"`
}

// ErrNotWSL2 is returned when Teleport is called outside a WSL2 environment.
var ErrNotWSL2 = fmt.Errorf("teleport is only available on WSL2 — you are already on native Linux")

// teleportFolders is the fixed set of Windows user folders to migrate.
var teleportFolders = []string{
	"Documents",
	"Desktop",
	"Downloads",
	"Pictures",
}

// Teleport walks C:\Users\<user> and symlinks Documents, Desktop, Downloads,
// and Pictures into the Linux home directory. Only runs on WSL2.
//
// dryRun=true prints what would happen without making any changes. The function
// returns a slice of TeleportResult, one per folder, describing the outcome.
//
// Security constraints:
//   - Symlink only (0 bytes allocated) — never copies or deletes data.
//   - Only operates on the 4 known user folders — no arbitrary path walk.
//   - Fails early with ErrNotWSL2 when called outside WSL2 context.
func Teleport(ctx context.Context, dryRun bool) ([]TeleportResult, error) {
	// Only supported on WSL2 — the Windows filesystem mount (/mnt/c/) is
	// only available inside WSL2. On native Linux there is nothing to migrate.
	if runtime.GOOS != "linux" {
		return nil, ErrNotWSL2
	}

	info, err := Probe(ctx)
	if err != nil {
		// Probe can return partial data with warnings — proceed anyway.
	}
	if !info.IsWSL2 {
		return nil, ErrNotWSL2
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	// Derive the Windows user profile path from the WSL2 mount.
	// Inside WSL2, C:\Users\<user> appears at /mnt/c/Users/<user>.
	// The hostname from WSL2 maps to the Windows username.
	hostname, hostErr := os.Hostname()
	if hostErr != nil {
		return nil, fmt.Errorf("cannot determine hostname for Windows path: %w", hostErr)
	}

	winHome := filepath.Join("/mnt", "c", "Users", hostname)

	// Verify the Windows home directory exists — if not, the mount may
	// be at a different path or WSL2 may not have been configured yet.
	if _, statErr := os.Stat(winHome); statErr != nil {
		return nil, fmt.Errorf("cannot access Windows user folder %q: %w", winHome, statErr)
	}

	results := make([]TeleportResult, 0, len(teleportFolders))

	for _, folder := range teleportFolders {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		source := filepath.Join(winHome, folder)
		target := filepath.Join(home, folder)

		// Check source exists
		srcInfo, srcErr := os.Stat(source)
		if srcErr != nil {
			if os.IsNotExist(srcErr) {
				results = append(results, TeleportResult{
					Source:  source,
					Target:  target,
					Skipped: true,
					Error:   "source folder does not exist",
				})
				continue
			}
			results = append(results, TeleportResult{
				Source: source,
				Target: target,
				Error:  fmt.Sprintf("cannot stat source: %v", srcErr),
			})
			continue
		}
		if !srcInfo.IsDir() {
			results = append(results, TeleportResult{
				Source: source,
				Target: target,
				Error:  "source path is not a directory",
			})
			continue
		}

		// Check if target already exists
		if _, targetErr := os.Lstat(target); targetErr == nil {
			results = append(results, TeleportResult{
				Source:        source,
				Target:        target,
				AlreadyExists: true,
			})
			continue
		}

		if dryRun {
			results = append(results, TeleportResult{
				Source: source,
				Target: target,
				Linked: false,
			})
			continue
		}

		// Create the symlink. This is a 0-byte operation — no data is copied
		// or moved. WSL2 resolves the target through the 9P protocol mount.
		if err := os.Symlink(source, target); err != nil {
			results = append(results, TeleportResult{
				Source: source,
				Target: target,
				Error:  fmt.Sprintf("symlink failed: %v", err),
			})
			continue
		}

		results = append(results, TeleportResult{
			Source: source,
			Target: target,
			Linked: true,
		})
	}

	return results, nil
}

// TeleportSummary returns a human-readable summary of teleport results.
func TeleportSummary(results []TeleportResult) string {
	var b strings.Builder
	b.WriteString("Teleport Results:\n")
	linked := 0
	already := 0
	skipped := 0
	errors := 0
	for _, r := range results {
		if r.Linked {
			linked++
			fmt.Fprintf(&b, "  ✓ %s → %s\n", r.Source, r.Target)
		} else if r.AlreadyExists {
			already++
			fmt.Fprintf(&b, "  ✓ %s already exists\n", r.Target)
		} else if r.Error != "" {
			errors++
			fmt.Fprintf(&b, "  ⚠ %s: %s\n", r.Source, r.Error)
		}
	}
	if linked == 0 && already == 0 && errors == 0 {
		fmt.Fprintf(&b, "  — (no action taken)\n")
	}
	_ = skipped
	fmt.Fprintf(&b, "\n  %d linked · %d already present · %d errors\n", linked, already, errors)
	return b.String()
}
