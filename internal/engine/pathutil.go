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
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveInNexusDir resolves symlinks for a file within the nexus base
// directory and ensures the resolved path has not escaped. This prevents
// symlink-following attacks where an attacker creates a symlink at
// ~/.nexus/state.json pointing to /etc/passwd (or any sensitive file).
//
// Threat model:
//   - An attacker with write access to ~/.nexus/ could replace state.json
//     with a symlink to an arbitrary file. filepath.EvalSymlinks would
//     follow it, causing the engine to read/write the target file.
//   - This function validates that the resolved path is still within
//     baseDir after symlink resolution.
func resolveInNexusDir(baseDir, filename string) (string, error) {
	original := filepath.Join(baseDir, filename)

	resolved, err := filepath.EvalSymlinks(original)
	if err != nil {
		// If the file doesn't exist yet, that's fine — we'll create it.
		// Only error on symlinks that actually resolve.
		if isNotExist(err) {
			return original, nil
		}
		return "", fmt.Errorf("failed to resolve path for %s: %w", filename, err)
	}

	// Ensure the resolved path is still within baseDir.
	// Use filepath.Rel to get a relative path from baseDir to resolved.
	// If the relative path starts with "..", it has escaped.
	rel, err := filepath.Rel(baseDir, resolved)
	if err != nil {
		return "", fmt.Errorf("SECURITY: cannot verify path for %s: %w", filename, err)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("SECURITY: %s symlink escapes nexus directory (resolved to %s)", filename, resolved)
	}

	return resolved, nil
}

// isNotExist checks if an error indicates a file does not exist.
func isNotExist(err error) bool {
	return os.IsNotExist(err)
}
