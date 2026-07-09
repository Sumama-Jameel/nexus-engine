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

package dotfiles

import (
	"os"
	"path/filepath"
)

// chezmoiConfigDir returns the Chezmoi config directory on Windows.
// Per Chezmoi docs: %USERPROFILE%\.config\chezmoi.
func chezmoiConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "chezmoi")
}

// chezmoiSourceDir returns the Chezmoi source directory on Windows.
// Per Chezmoi docs: %LOCALAPPDATA%\chezmoi. Falls back to
// %USERPROFILE%\AppData\Local\chezmoi if LOCALAPPDATA is unset.
func chezmoiSourceDir() string {
	if appdata := os.Getenv("LOCALAPPDATA"); appdata != "" {
		return filepath.Join(appdata, "chezmoi")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "AppData", "Local", "chezmoi")
}
