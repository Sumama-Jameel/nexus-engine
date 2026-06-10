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

// DnfInstaller implements PackageManager for Fedora/RHEL systems.
type DnfInstaller struct {
	execFn ExecFunc
}

// Name returns the identifier for this package manager: "dnf".
func (d *DnfInstaller) Name() string { return "dnf" }

// RefreshIndex runs `sudo dnf makecache` to refresh the metadata.
// Without this, dnf may use stale repository data.
func (d *DnfInstaller) RefreshIndex(ctx context.Context) error {
	_, err := d.execFn(ctx, "sudo", "dnf", "makecache")
	return err
}

// Install executes `sudo dnf install -y <pkg>` for each package.
// Per the Orchestrator's rules: packages are installed individually so each
// gets its own success/failure result.
func (d *DnfInstaller) Install(ctx context.Context, packages []string) ([]PackageResult, error) {
	var results []PackageResult
	for _, pkg := range packages {
		start := time.Now()
		result := PackageResult{Package: pkg, Action: "install"}
		_, err := d.execFn(ctx, "sudo", "dnf", "install", "-y", pkg)
		result.Duration = time.Since(start)
		if err != nil {
			result.Success = false
			result.Error = classifyDnfError(err.Error())
		} else {
			result.Success = true
		}
		results = append(results, result)
	}
	return results, nil
}

// Remove executes `sudo dnf remove -y <pkg>` for each package.
func (d *DnfInstaller) Remove(ctx context.Context, packages []string) ([]PackageResult, error) {
	var results []PackageResult
	for _, pkg := range packages {
		start := time.Now()
		result := PackageResult{Package: pkg, Action: "remove"}
		_, err := d.execFn(ctx, "sudo", "dnf", "remove", "-y", pkg)
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

// Update executes `sudo dnf upgrade -y <pkg>` for each package.
func (d *DnfInstaller) Update(ctx context.Context, packages []string) ([]PackageResult, error) {
	var results []PackageResult
	for _, pkg := range packages {
		start := time.Now()
		result := PackageResult{Package: pkg, Action: "update"}
		_, err := d.execFn(ctx, "sudo", "dnf", "upgrade", "-y", pkg)
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

// IsInstalled checks via `rpm -q <pkg>`.
func (d *DnfInstaller) IsInstalled(ctx context.Context, pkg string) bool {
	_, err := d.execFn(ctx, "rpm", "-q", pkg)
	return err == nil
}

// ListInstalled returns all installed packages via `rpm -qa`.
func (d *DnfInstaller) ListInstalled(ctx context.Context) ([]string, error) {
	output, err := d.execFn(ctx, "rpm", "-qa")
	if err != nil {
		return nil, fmt.Errorf("failed to list installed packages: %w", err)
	}
	var packages []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			packages = append(packages, line)
		}
	}
	return packages, nil
}

// Search searches via `dnf search`.
func (d *DnfInstaller) Search(ctx context.Context, query string) ([]string, error) {
	output, err := d.execFn(ctx, "dnf", "search", query)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	var results []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "Last metadata") && !strings.HasPrefix(line, "=") {
			// dnf search returns "package.arch : description"
			parts := strings.SplitN(line, ".", 2)
			if len(parts) >= 1 {
				results = append(results, strings.TrimSpace(parts[0]))
			}
		}
	}
	return results, nil
}

// classifyDnfError maps raw dnf errors to structured error categories.
// This enables the Orchestrator to decide whether to retry, skip, or abort.
func classifyDnfError(raw string) string {
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "no match for argument"):
		return "PACKAGE_NOT_FOUND"
	case strings.Contains(lower, "permission denied"):
		return "NO_SUDO"
	case strings.Contains(lower, "already installed"):
		return "ALREADY_INSTALLED"
	default:
		return "UNKNOWN"
	}
}
