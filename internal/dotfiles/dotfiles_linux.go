//go:build linux

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
	"os"
	"path/filepath"
)

// chezmoiConfigDir returns the Chezmoi config directory on Linux.
// Per Chezmoi docs: $XDG_CONFIG_HOME/chezmoi (defaults to ~/.config/chezmoi).
// We honor XDG_CONFIG_HOME when set; otherwise fall back to ~/.config.
func chezmoiConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "chezmoi")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "chezmoi")
}

// chezmoiSourceDir returns the Chezmoi source directory on Linux.
// Per Chezmoi docs: $XDG_DATA_HOME/chezmoi (defaults to ~/.local/share/chezmoi).
func chezmoiSourceDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "chezmoi")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "chezmoi")
}
