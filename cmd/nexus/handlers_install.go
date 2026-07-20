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

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Sumama-Jameel/nexus-engine/internal/bridge"
	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
	"github.com/spf13/cobra"
)

func runInstall(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	rdeps, err := initRunnerDeps(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = rdeps.Audit.Close() }()

	// Handle file-path profile fallback (not supported by runner — CLI-only)
	profileName := profilePath
	var packages []string
	if profileName != "" {
		// Try runner's profile resolution first
		if rdeps.ProfileStore != nil {
			profile, pkgs, resolveErr := rdeps.ResolveProfilePackages(profileName)
			if resolveErr == nil {
				packages = pkgs
				profileName = profile.Name
			}
		}

		// Fallback: try as file path
		if len(packages) == 0 {
			manifestFile := profileName
			if _, statErr := os.Stat(manifestFile); statErr != nil {
				searchPaths := []string{
					profileName,
					"/etc/nexus/" + profileName + ".yaml",
				}
				for _, p := range searchPaths {
					if _, sErr := os.Stat(p); sErr == nil {
						manifestFile = p
						break
					}
				}
			}

			parsed, parseErr := manifest.Parse(manifestFile)
			if parseErr != nil {
				return fmt.Errorf("failed to load profile '%s': %w", profileName, parseErr)
			}

			env := bridge.DetectEnvironment(ctx)
			target := manifest.ResolveTarget(parsed, env.PackageManager)
			if target == nil {
				return fmt.Errorf("no compatible target found in profile '%s'", profileName)
			}
			packages = target.Packages
			profileName = parsed.Name
		}
	} else {
		packages = args
	}

	if !outputJSON {
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════════════════╗")
		fmt.Println("  ║       NEXUS PROTOCOL — PACKAGE INSTALLER         ║")
		fmt.Println("  ╚══════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Printf("  📦 Package Manager: %s\n", rdeps.PM.Name())
		fmt.Printf("  📋 Packages: %d\n", len(packages))
		if profileName != "" && profileName != "cli" {
			fmt.Printf("  📄 Profile: %s\n", profileName)
		}
		fmt.Println()
	}

	result, _, installErr := rdeps.InstallPackages(ctx, packages, profileName)

	if outputJSON {
		return jsonOutput(result)
	}

	if installErr != nil {
		fmt.Fprintf(os.Stderr, "  ⛔ %v\n", installErr)
	}

	fmt.Println(installer.FormatOrchestratorResult(result))

	if result != nil && !result.Aborted && result.Failed == 0 && !dryRun {
		fmt.Println("  ✅ All packages installed and verified successfully")
	}

	return nil
}

func runRemove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	rdeps, err := initRunnerDeps(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = rdeps.Audit.Close() }()

	if !outputJSON {
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════════════════╗")
		fmt.Println("  ║        NEXUS PROTOCOL — PACKAGE REMOVER          ║")
		fmt.Println("  ╚══════════════════════════════════════════════════╝")
		fmt.Println()
	}

	result, _ := rdeps.RemovePackages(ctx, args)

	if len(result.NotManaged) > 0 && !outputJSON {
		fmt.Printf("  ⚠️  Not managed by Nexus (skipped): %v\n", result.NotManaged)
	}

	if len(result.DependencyWarnings) > 0 && !outputJSON {
		fmt.Println("  ── DEPENDENCY WARNINGS ──────────────────────────")
		for _, w := range result.DependencyWarnings {
			fmt.Printf("  ⚠️  %s\n", w)
		}
		fmt.Println()
	}

	if len(result.ToRemove) == 0 {
		if !outputJSON {
			fmt.Println("  No Nexus-managed packages to remove")
		}
		return nil
	}

	if !outputJSON {
		for _, r := range result.PackageResults {
			if r.Success {
				fmt.Printf("  ✅ Removed: %s\n", r.Package)
			} else {
				fmt.Printf("  ⛔ Failed: %s — %s\n", r.Package, r.Error)
			}
		}
	}

	if outputJSON {
		return jsonOutput(map[string]interface{}{
			"removed": result.ToRemove, "not_managed": result.NotManaged,
			"dependency_warnings": result.DependencyWarnings, "results": result.PackageResults,
		})
	}
	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	rdeps, err := initRunnerDeps(ctx)
	if err != nil {
		return err
	}

	managed, _ := rdeps.ListManagedPackages()

	if outputJSON {
		return jsonOutput(managed)
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║      NEXUS PROTOCOL — MANAGED PACKAGES          ║")
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	if len(managed) == 0 {
		fmt.Println("  No Nexus-managed packages found.")
		fmt.Println("  Run 'nexus init' or 'nexus install --profile base-dev' to get started.")
		return nil
	}

	fmt.Printf("  %-20s %-12s %-10s %s\n", "PACKAGE", "MANAGER", "VERIFIED", "INSTALLED")
	fmt.Println("  " + strings.Repeat("─", 65))
	for pkg, s := range managed {
		verified := "✅"
		if !s.Verified {
			verified = "⛔"
		}
		fmt.Printf("  %-20s %-12s %-10s %s\n", pkg, s.PackageManager, verified,
			s.InstalledAt.Format("2006-01-02 15:04"))
	}
	fmt.Printf("\n  Total: %d packages | Profiles: %v\n", len(managed), rdeps.State.GetProfiles())
	fmt.Println()
	return nil
}

func runSearch(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	rdeps, err := initRunnerDeps(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = rdeps.Audit.Close() }()

	results, err := rdeps.SearchPackages(ctx, args[0])
	if err != nil {
		return err
	}

	if outputJSON {
		return jsonOutput(results)
	}

	fmt.Println()
	fmt.Printf("  Search results for '%s' (via %s):\n\n", args[0], rdeps.PM.Name())
	if len(results) == 0 {
		fmt.Println("  No packages found.")
		return nil
	}
	limit := 20
	if len(results) < limit {
		limit = len(results)
	}
	for i, pkg := range results[:limit] {
		fmt.Printf("  %2d. %s\n", i+1, pkg)
	}
	if len(results) > 20 {
		fmt.Printf("\n  ... and %d more results\n", len(results)-20)
	}
	fmt.Println()
	return nil
}

func runUpdate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	rdeps, err := initRunnerDeps(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = rdeps.Audit.Close() }()

	var packages []string
	if len(args) > 0 {
		packages = args
	}

	result, _ := rdeps.UpdatePackages(ctx, packages)

	if len(result.Packages) == 0 {
		if !outputJSON {
			fmt.Println("  No packages to update")
		}
		return nil
	}

	if !outputJSON {
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════════════════╗")
		fmt.Println("  ║       NEXUS PROTOCOL — PACKAGE UPDATER           ║")
		fmt.Println("  ╚══════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Printf("  📦 Updating %d packages via %s...\n\n", len(result.Packages), rdeps.PM.Name())

		succeeded, failed := 0, 0
		for _, r := range result.PackageResults {
			if r.Success {
				succeeded++
				fmt.Printf("  ✅ Updated: %s\n", r.Package)
			} else {
				failed++
				fmt.Printf("  ⛔ Failed: %s — %s\n", r.Package, r.Error)
			}
		}
		fmt.Printf("\n  Updated: %d | Failed: %d\n", succeeded, failed)
	}

	if outputJSON {
		return jsonOutput(map[string]interface{}{"packages": result.Packages, "results": result.PackageResults})
	}
	return nil
}
