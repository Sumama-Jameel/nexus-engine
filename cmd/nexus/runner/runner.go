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

// Package runner implements the business logic for Nexus CLI commands.
// Per the Humble Object pattern: the runner holds the business logic,
// main.go is just the thin CLI wiring/formatting layer.
//
// This package uses dependency injection — all external dependencies
// (PackageManager, StateTracker, AuditLogger, ProfileStore, ExecFunc)
// are injected via the Dependencies struct, making the runner fully
// testable with mocks.
//
// Security model: this package does NOT import os/exec or call
// exec.Command directly. All command execution goes through the
// injected ExecFunc, which is wired to engine.SanitizeAndExecute
// (Zero-Trust boundary).
package runner

import (
	"os"

	"github.com/Sumama-Jameel/nexus-engine/internal/bridge"
	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
)

// Dependencies holds all injected dependencies for runner methods.
// This is the Composition Root — all concrete implementations are
// injected from main.go, ensuring the runner has no global state,
// no init() functions, and no direct OS dependencies.
type Dependencies struct {
	// PM is the package manager implementation (apt, pacman, dnf, apk).
	PM installer.PackageManager

	// State persists installation records to ~/.nexus/state.json.
	State *engine.StateTracker

	// Audit provides append-only audit logging to ~/.nexus/audit.log.
	Audit *engine.AuditLogger

	// Env contains the detected environment information (OS, WSL2, etc.).
	Env *bridge.EnvironmentInfo

	// Family is the OS family identifier (e.g., "debian", "arch", "fedora", "alpine").
	Family string

	// ExecFn is the security-gated command execution function.
	// MUST be wired to engine.SanitizeAndExecute for Zero-Trust compliance.
	ExecFn installer.ExecFunc

	// ProfileStore manages the local profile directory at ~/.nexus/profiles/.
	// May be nil if profile operations are not available.
	ProfileStore *manifest.ProfileStore

	// Output is the target for non-JSON output (typically os.Stdout).
	Output *os.File

	// JSONOutput indicates whether the --json flag is set.
	JSONOutput bool

	// DryRun indicates whether the --dry-run flag is set.
	DryRun bool

	// ForceRemove indicates whether the --force flag is set.
	ForceRemove bool
}

// RemoveResult is the structured outcome of a package removal operation.
type RemoveResult struct {
	// ToRemove lists packages that were or are ready to be removed.
	ToRemove []string

	// NotManaged lists packages that are not tracked by Nexus (skipped).
	NotManaged []string

	// DependencyWarnings lists warnings about dependent packages that
	// may be affected by the removal.
	DependencyWarnings []string

	// PackageResults contains per-package removal outcomes.
	PackageResults []installer.PackageResult
}

// UpdateResult is the structured outcome of a package update operation.
type UpdateResult struct {
	// Packages lists the packages that were updated.
	Packages []string

	// PackageResults contains per-package update outcomes.
	PackageResults []installer.PackageResult
}
