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

// AptInstaller implements PackageManager for Debian/Ubuntu systems.
// Uses apt-get and dpkg as the underlying commands.
// Every command passes through the ExecFunc (SanitizeAndExecute) —
// the installer never calls exec.Command directly.
type AptInstaller struct {
	execFn ExecFunc
}

// Name returns the identifier for this package manager: "apt".
func (a *AptInstaller) Name() string { return "apt" }

// RefreshIndex runs `sudo apt-get update` to refresh the package index.
// Without this, apt-get install may fail with "Unable to locate package"
// on freshly provisioned systems where the cache has never been populated.
func (a *AptInstaller) RefreshIndex(ctx context.Context) error {
	_, err := a.execFn(ctx, "sudo", "apt-get", "update")
	return err
}

// Install executes `sudo apt-get install -y <packages>` for each package.
// Per the Orchestrator's rules: we install packages individually so we
// get per-package success/failure, not an all-or-nothing batch result.
func (a *AptInstaller) Install(ctx context.Context, packages []string) ([]PackageResult, error) {
	var results []PackageResult

	for _, pkg := range packages {
		start := time.Now()
		result := PackageResult{
			Package: pkg,
			Action:  "install",
		}

		output, err := a.execFn(ctx, "sudo", "apt-get", "install", "-y", pkg)
		result.Duration = time.Since(start)

		if err != nil {
			result.Success = false
			result.Error = classifyAptError(err.Error())
		} else {
			result.Success = true
			_ = output
		}

		results = append(results, result)
	}

	return results, nil
}

// Remove executes `sudo apt-get remove -y <packages>`.
func (a *AptInstaller) Remove(ctx context.Context, packages []string) ([]PackageResult, error) {
	var results []PackageResult

	for _, pkg := range packages {
		start := time.Now()
		result := PackageResult{
			Package: pkg,
			Action:  "remove",
		}

		_, err := a.execFn(ctx, "sudo", "apt-get", "remove", "-y", pkg)
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

// Update executes `sudo apt-get install --only-upgrade -y <packages>`.
// Note: RefreshIndex should be called before Update for best results.
func (a *AptInstaller) Update(ctx context.Context, packages []string) ([]PackageResult, error) {
	var results []PackageResult

	for _, pkg := range packages {
		start := time.Now()
		result := PackageResult{
			Package: pkg,
			Action:  "update",
		}

		_, err := a.execFn(ctx, "sudo", "apt-get", "install", "--only-upgrade", "-y", pkg)
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

// IsInstalled checks if a package is installed via `dpkg -s`.
// Returns true only if the package status contains "install ok installed".
func (a *AptInstaller) IsInstalled(ctx context.Context, pkg string) bool {
	output, err := a.execFn(ctx, "dpkg", "-s", pkg)
	if err != nil {
		return false
	}
	return strings.Contains(output, "Status: install ok installed")
}

// ListInstalled returns all packages with "install" status via `dpkg --get-selections`.
func (a *AptInstaller) ListInstalled(ctx context.Context) ([]string, error) {
	output, err := a.execFn(ctx, "dpkg", "--get-selections")
	if err != nil {
		return nil, fmt.Errorf("failed to list installed packages: %w", err)
	}

	var packages []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "install" {
			packages = append(packages, fields[0])
		}
	}
	return packages, nil
}

// Search searches for packages via `apt-cache search`.
func (a *AptInstaller) Search(ctx context.Context, query string) ([]string, error) {
	output, err := a.execFn(ctx, "apt-cache", "search", query)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	var results []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			// apt-cache search returns "package - description"
			pkg := strings.SplitN(line, " - ", 2)[0]
			results = append(results, pkg)
		}
	}
	return results, nil
}

// classifyAptError maps raw apt errors to structured error categories.
// This is the "intelligent error parsing" the Orchestrator uses to
// decide whether to retry, skip, or abort.
func classifyAptError(raw string) string {
	lower := strings.ToLower(raw)

	switch {
	case strings.Contains(lower, "unable to locate package"):
		return "PACKAGE_NOT_FOUND"
	case strings.Contains(lower, "dpkg was interrupted"):
		return "SYSTEM_BROKEN"
	case strings.Contains(lower, "permission denied"):
		return "NO_SUDO"
	case strings.Contains(lower, "could not get lock"):
		return "LOCK_HELD"
	case strings.Contains(lower, "unmet dependencies"):
		return "DEPENDENCY_ERROR"
	case strings.Contains(lower, "no space left"):
		return "DISK_FULL"
	case strings.Contains(lower, "failed to fetch"):
		return "NETWORK_ERROR"
	default:
		return "UNKNOWN"
	}
}
