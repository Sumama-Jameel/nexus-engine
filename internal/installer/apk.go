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

// ApkInstaller implements PackageManager for Alpine Linux systems.
// Alpine typically runs as root, so no sudo is needed.
type ApkInstaller struct {
        execFn ExecFunc
}

// Name returns the identifier for this package manager: "apk".
func (a *ApkInstaller) Name() string { return "apk" }

// RefreshIndex runs `apk update` to refresh the package index.
func (a *ApkInstaller) RefreshIndex(ctx context.Context) error {
        _, err := a.execFn(ctx, "apk", "update")
        return err
}

// Install executes `apk add <pkg>` for each package.
// Per the Orchestrator's rules: packages are installed individually so each
// gets its own success/failure result.
func (a *ApkInstaller) Install(ctx context.Context, packages []string) ([]PackageResult, error) {
        var results []PackageResult
        for _, pkg := range packages {
                start := time.Now()
                result := PackageResult{Package: pkg, Action: "install"}
                _, err := a.execFn(ctx, "apk", "add", pkg)
                result.Duration = time.Since(start)
                if err != nil {
                        result.Success = false
                        result.Error = classifyApkError(err.Error())
                } else {
                        result.Success = true
                }
                results = append(results, result)
        }
        return results, nil
}

// Remove executes `apk del <pkg>` for each package.
func (a *ApkInstaller) Remove(ctx context.Context, packages []string) ([]PackageResult, error) {
        var results []PackageResult
        for _, pkg := range packages {
                start := time.Now()
                result := PackageResult{Package: pkg, Action: "remove"}
                _, err := a.execFn(ctx, "apk", "del", pkg)
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

// Update executes `apk upgrade <pkg>` for each package.
func (a *ApkInstaller) Update(ctx context.Context, packages []string) ([]PackageResult, error) {
        var results []PackageResult
        for _, pkg := range packages {
                start := time.Now()
                result := PackageResult{Package: pkg, Action: "update"}
                _, err := a.execFn(ctx, "apk", "upgrade", pkg)
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

// IsInstalled checks via `apk info -e <pkg>`.
func (a *ApkInstaller) IsInstalled(ctx context.Context, pkg string) bool {
        output, err := a.execFn(ctx, "apk", "info", "-e", pkg)
        if err != nil {
                return false
        }
        return strings.TrimSpace(output) != ""
}

// ListInstalled returns all installed packages via `apk info`.
func (a *ApkInstaller) ListInstalled(ctx context.Context) ([]string, error) {
        output, err := a.execFn(ctx, "apk", "info")
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

// Search searches via `apk search`.
func (a *ApkInstaller) Search(ctx context.Context, query string) ([]string, error) {
        output, err := a.execFn(ctx, "apk", "search", query)
        if err != nil {
                return nil, fmt.Errorf("search failed: %w", err)
        }
        var results []string
        for _, line := range strings.Split(output, "\n") {
                line = strings.TrimSpace(line)
                if line != "" {
                        results = append(results, line)
                }
        }
        return results, nil
}

// classifyApkError maps raw apk errors to structured error categories.
// This enables the Orchestrator to decide whether to retry, skip, or abort.
func classifyApkError(raw string) string {
        lower := strings.ToLower(raw)
        switch {
        case strings.Contains(lower, "could not find"):
                return "PACKAGE_NOT_FOUND"
        case strings.Contains(lower, "permission denied"):
                return "NO_SUDO"
        default:
                return "UNKNOWN"
        }
}
