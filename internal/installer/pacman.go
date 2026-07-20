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
	"time"
)

// PacmanInstaller implements PackageManager for Arch Linux systems.
type PacmanInstaller struct {
	execFn ExecFunc
}

// Name returns the identifier for this package manager: "pacman".
func (p *PacmanInstaller) Name() string { return "pacman" }

// RefreshIndex runs `sudo pacman -Sy` to refresh the package database.
// On Arch, this is critical because Arch is a rolling release —
// the local cache can be hours out of date.
func (p *PacmanInstaller) RefreshIndex(ctx context.Context) error {
	_, err := p.execFn(ctx, "sudo", "pacman", "-Sy")
	return err
}

// Install executes `sudo pacman -S --noconfirm <pkg>` for each package.
// Per the Orchestrator's rules: packages are installed individually so each
// gets its own success/failure result.
func (p *PacmanInstaller) Install(ctx context.Context, packages []string) ([]PackageResult, error) {
	var results []PackageResult
	for _, pkg := range packages {
		start := time.Now()
		result := PackageResult{Package: pkg, Action: "install"}
		_, err := p.execFn(ctx, "sudo", "pacman", "-S", "--noconfirm", pkg)
		result.Duration = time.Since(start)
		if err != nil {
			result.Success = false
			result.Error = classifyPacmanError(err.Error())
		} else {
			result.Success = true
		}
		results = append(results, result)
	}
	return results, nil
}

// Remove executes `sudo pacman -R --noconfirm <pkg>` for each package.
func (p *PacmanInstaller) Remove(ctx context.Context, packages []string) ([]PackageResult, error) {
	var results []PackageResult
	for _, pkg := range packages {
		start := time.Now()
		result := PackageResult{Package: pkg, Action: "remove"}
		_, err := p.execFn(ctx, "sudo", "pacman", "-R", "--noconfirm", pkg)
		result.Duration = time.Since(start)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else {
			result.Success = true
		}
		results = append(results, result)
	}
	return results, nil
}

// Update executes `sudo pacman -S --noconfirm <pkg>` for each package.
// On Arch's rolling release model, reinstalling with -S effectively upgrades.
func (p *PacmanInstaller) Update(ctx context.Context, packages []string) ([]PackageResult, error) {
	var results []PackageResult
	for _, pkg := range packages {
		start := time.Now()
		result := PackageResult{Package: pkg, Action: "update"}
		_, err := p.execFn(ctx, "sudo", "pacman", "-S", "--noconfirm", pkg)
		result.Duration = time.Since(start)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
		} else {
			result.Success = true
		}
		results = append(results, result)
	}
	return results, nil
}

// IsInstalled checks via `pacman -Q <pkg>`.
func (p *PacmanInstaller) IsInstalled(ctx context.Context, pkg string) bool {
	_, err := p.execFn(ctx, "pacman", "-Q", pkg)
	return err == nil
}

// ListInstalled returns all explicitly installed packages via `pacman -Qe`.
func (p *PacmanInstaller) ListInstalled(ctx context.Context) ([]string, error) {
	output, err := p.execFn(ctx, "pacman", "-Qe")
	if err != nil {
		return nil, fmt.Errorf("failed to list installed packages: %w", err)
	}
	var packages []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			packages = append(packages, fields[0])
		}
	}
	return packages, nil
}

// Search searches via `pacman -Ss`.
func (p *PacmanInstaller) Search(ctx context.Context, query string) ([]string, error) {
	output, err := p.execFn(ctx, "pacman", "-Ss", query)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	var results []string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "  ") {
			continue // skip description lines
		}
		// Format: "repo/package version"
		parts := strings.SplitN(line, "/", 2)
		if len(parts) >= 2 {
			pkgParts := strings.Fields(parts[1])
			if len(pkgParts) >= 1 {
				results = append(results, pkgParts[0])
			}
		}
	}
	return results, nil
}

// classifyPacmanError maps raw pacman errors to structured error categories.
// This enables the Orchestrator to decide whether to retry, skip, or abort.
func classifyPacmanError(raw string) string {
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "target not found"):
		return "PACKAGE_NOT_FOUND"
	case strings.Contains(lower, "permission denied"):
		return "NO_SUDO"
	case strings.Contains(lower, "failed to commit transaction"):
		return "TRANSACTION_ERROR"
	case strings.Contains(lower, "exists in filesystem"):
		return "FILE_CONFLICT"
	default:
		return "UNKNOWN"
	}
}
