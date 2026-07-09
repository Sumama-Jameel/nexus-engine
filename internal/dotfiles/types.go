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

// Package dotfiles is the V7 bounded context for managing user configuration
// (shell, editor, aliases) via Chezmoi. The package name reflects the domain,
// not the underlying tool — Chezmoi is the implementation, dotfiles is the
// concept.
//
// Per V7 "The Chezmoi Integration (The Memory)":
// "This allows the user's settings (terminal theme, shortcuts) to move
// between machines."
//
// Per Zero-Trust: every command in this package flows through a caller-injected
// ExecFunc (typically engine.SanitizeAndExecute). The package never calls
// exec.Command directly. This makes the package trivially testable and
// enforces the same security boundary as the rest of the engine.
package dotfiles

import "time"

// DetectReport captures the result of probing the host for a Chezmoi
// installation. It is intentionally minimal — only fields reliably known
// after a single `chezmoi --version` call.
//
// When Installed is false, all other fields are zero-valued and should be
// treated as absent.
type DetectReport struct {
	// Installed reports whether the chezmoi binary was found on PATH.
	Installed bool `json:"installed"`
	// Version is the parsed semver from `chezmoi --version` output
	// (e.g., "2.50.0"). Empty when Installed is false.
	Version string `json:"version,omitempty"`
	// ConfigDir is the absolute path to Chezmoi's config directory.
	// Linux:   $HOME/.config/chezmoi
	// Windows: $USERPROFILE/.config/chezmoi
	ConfigDir string `json:"config_dir,omitempty"`
	// SourceDir is the absolute path to Chezmoi's source directory
	// (where managed file templates live before being applied).
	// Linux:   $HOME/.local/share/chezmoi
	// Windows: %LOCALAPPDATA%\chezmoi
	SourceDir string `json:"source_dir,omitempty"`
	// ProbedAt is the UTC timestamp of the probe.
	ProbedAt time.Time `json:"probed_at"`
}
