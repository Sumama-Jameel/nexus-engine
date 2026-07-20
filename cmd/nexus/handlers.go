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
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/Sumama-Jameel/nexus-engine/internal/bridge"
	"github.com/Sumama-Jameel/nexus-engine/internal/container"
	"github.com/Sumama-Jameel/nexus-engine/internal/dotfiles"
	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
	"github.com/Sumama-Jameel/nexus-engine/internal/ledger"
	"github.com/Sumama-Jameel/nexus-engine/internal/mode"
	"github.com/Sumama-Jameel/nexus-engine/internal/wsl"
	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
	"github.com/spf13/cobra"
)

func runProbe(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cfg, _ := engine.InitConfig()
	_ = cfg

	info, err := engine.Probe(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Probe completed with warnings: %v\n", err)
	}

	if outputJSON {
		return jsonOutput(info)
	}

	fmt.Print(engine.FormatSystemInfo(info))
	env := bridge.DetectEnvironment(ctx)
	fmt.Print(bridge.FormatEnvironmentInfo(env))
	return nil
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cfg, _ := engine.InitConfig()
	_ = cfg

	if !outputJSON {
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════════════════╗")
		fmt.Println("  ║         NEXUS PROTOCOL — INITIALIZING            ║")
		fmt.Println("  ╚══════════════════════════════════════════════════════╝")
		fmt.Println()
	}

	// STEP 1: PROBE
	if !outputJSON {
		fmt.Println("  [1/5] 🔍 Probing system...")
	}
	info, _ := engine.Probe(ctx)

	// STEP 2: VALIDATE
	if !outputJSON {
		fmt.Println("  [2/5] ✅ Validating prerequisites...")
	}
	env := bridge.DetectEnvironment(ctx)

	// STEP 3: APPLY
	if !outputJSON {
		fmt.Println("  [3/5] 📋 Loading manifest and applying packages via Orchestrator...")
	}

	// Initialize profile store
	store, storeErr := initProfileStore()
	if storeErr != nil && !outputJSON {
		fmt.Fprintf(os.Stderr, "  ⚠️  Profile store init failed: %v\n", storeErr)
	}

	var profile *manifest.NexusProfile
	var orchResult *installer.OrchestratorResult
	profileName := "base-dev"

	if initConfigPath != "" {
		// Custom config path specified
		parsed, manifestErr := manifest.Parse(initConfigPath)
		if manifestErr != nil {
			if !outputJSON {
				fmt.Fprintf(os.Stderr, "  ⛔ Manifest error: %v\n", manifestErr)
			}
		} else {
			profile = parsed
			profileName = parsed.Name
		}
	} else if store != nil {
		// Load from profile store (with extends resolution)
		resolved, resolveErr := store.LoadProfileWithExtends(profileName)
		if resolveErr != nil {
			if !outputJSON {
				fmt.Fprintf(os.Stderr, "  ⚠️  Profile load error: %v\n", resolveErr)
			}
			// No fallback — the profile store is the single source of truth.
			// Embedded defaults are always seeded to the store on first run.
		} else {
			profile = resolved
			if !outputJSON {
				fmt.Printf("  ✅ Profile loaded: %s v%s\n", resolved.Name, resolved.Version)
				if resolved.Extends != "" {
					fmt.Printf("  🧬 Resolved extends: %s\n", resolved.Extends)
				}
			}
		}
	}

	if profile != nil {
		deps, depsErr := initDeps(ctx)
		if depsErr != nil {
			if !outputJSON {
				fmt.Fprintf(os.Stderr, "  ⛔ Installer init failed: %v\n", depsErr)
			}
		} else {
			defer deps.audit.Close()

			target := manifest.ResolveTarget(profile, env.PackageManager)
			if target != nil {
				if !outputJSON {
					fmt.Printf("  📦 Resolved target: %s (%d packages)\n", target.Family, len(target.Packages))
				}

				orch := installer.NewOrchestrator(deps.pm, deps.execFn, deps.state, deps.audit, profileName, dryRun)
				orchResult, _ = orch.Install(ctx, target.Packages)

				if !outputJSON && orchResult != nil {
					if dryRun {
						fmt.Printf("  🔄 [DRY RUN] Would install %d packages via %s\n", orchResult.Total, deps.pm.Name())
					} else {
						fmt.Print(installer.FormatOrchestratorResult(orchResult))
					}
				}

				// Record profile as applied
				if store != nil && !dryRun {
					store.RecordApplied(profileName)
				}
			}
		}
	}

	// STEP 4: CONFIGURE (with profile env vars)
	if !outputJSON {
		fmt.Println("  [4/5] 🧬 Configuring environment...")
	}

	envVars := make(map[string]string)
	if profile != nil {
		for k, v := range profile.Env {
			envVars[k] = v
		}
	}
	// Always set NEXUS_PROFILE
	envVars["NEXUS_PROFILE"] = profileName

	configureResult := engine.Configure(ctx, envVars)
	if !outputJSON {
		for _, msg := range configureResult.Messages {
			fmt.Printf("    %s\n", msg)
		}
	}

	// STEP 4b: DOTFILES (V7) — Apply the profile's dotfiles section if present.
	// Per V7 plan: the Configure step initializes Chezmoi (above); this sub-step
	// binds the source, applies dotfiles (if requested), and tracks managed paths.
	// Failures here are warnings, not fatal — init should not abort on dotfile issues.
	if profile != nil && profile.Dotfiles != nil && !dryRun {
		if !outputJSON {
			fmt.Println("  [4b] 🧬 Applying dotfiles from profile...")
		}
		deps, depsErr := initDeps(ctx)
		if depsErr == nil {
			profileDeps := dotfiles.ProfileDeps{
				ExecFn: deps.execFn,
				State:  deps.state,
				Audit:  deps.audit,
			}
			dotReport := dotfiles.ApplyFromProfile(ctx, profile, profileDeps)
			if !outputJSON {
				if dotReport.SourceBound {
					fmt.Println("    ✅ Source bound")
				}
				if dotReport.Applied {
					fmt.Println("    ✅ Dotfiles applied")
				}
				for _, p := range dotReport.AddedPaths {
					fmt.Printf("    ➕ Tracking: %s\n", p)
				}
				for _, p := range dotReport.SkippedPaths {
					fmt.Printf("    ⏭️  Skipped (sensitive): %s\n", p)
				}
				for _, w := range dotReport.Warnings {
					fmt.Printf("    ⚠️  %s\n", w)
				}
			}
		}
	}

	// STEP 5: REPORT
	if !outputJSON {
		fmt.Println("  [5/6] 📊 Generating report...")
		fmt.Println()
		fmt.Print(engine.FormatSystemInfo(info))
		fmt.Print(bridge.FormatEnvironmentInfo(env))

		if profile != nil {
			fmt.Println("  ── MANIFEST ────────────────────────────────────")
			fmt.Printf("  📄 Profile:       %s v%s\n", profile.Name, profile.Version)
			fmt.Printf("  📝 Description:   %s\n", profile.Description)
			if profile.Extends != "" {
				fmt.Printf("  🧬 Extends:       %s\n", profile.Extends)
			}
		}

		fmt.Println("  ── CONFIGURATION ──────────────────────────────")
		fmt.Printf("  📁 Nexus Dir:     %s\n", configureResult.NexusDir)
		fmt.Printf("  🐚 Shell:         %s\n", configureResult.ShellType)
		fmt.Printf("  🔑 Env Vars:      %d applied from profile\n", configureResult.EnvVarsApplied)
		if configureResult.ChezmoiInstalled {
			fmt.Printf("  🧬 Chezmoi:       Installed\n")
		} else {
			fmt.Println("  🧬 Chezmoi:       Not installed")
		}
		if cfg != nil {
			fmt.Printf("  ⚙️  Config:        %s\n", engine.FormatConfig(cfg))
		}
		fmt.Println()
	} else {
		result := map[string]interface{}{
			"status": "ok", "system": info, "environment": env, "configure": configureResult,
		}
		if orchResult != nil {
			result["orchestrator"] = orchResult
		}
		if profile != nil {
			result["profile"] = profile
		}
		return jsonOutput(result)
	}

	// STEP 6: LEDGER RECORD
	if ledgerState, ledgerErr := engine.NewStateTracker(); ledgerErr == nil {
		if err := ledger.RecordSimple(ctx, ledgerState); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠️  Ledger record failed: %v\n", err)
		}
	}

	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║   ✅ NEXUS INIT COMPLETE — SYSTEM READY          ║")
	fmt.Println("  ╚═══════════════════════════════════════════════════╝")
	fmt.Println()

	return nil
}

// ─── V2 Commands ───

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

// ─── V3 Commands: Profile Management ───

func runProfileList(cmd *cobra.Command, args []string) error {
	store, err := initProfileStore()
	if err != nil {
		return err
	}

	profiles := store.ListProfiles()

	if outputJSON {
		return jsonOutput(profiles)
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║       NEXUS PROTOCOL — PROFILE REGISTRY         ║")
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	if len(profiles) == 0 {
		fmt.Println("  No profiles found. Run 'nexus init' to initialize defaults.")
		return nil
	}

	fmt.Printf("  %-20s %-10s %-8s %-20s %s\n", "NAME", "SOURCE", "VERSION", "SHA256", "LAST APPLIED")
	fmt.Println("  " + strings.Repeat("─", 75))
	for _, meta := range profiles {
		lastApplied := "never"
		if meta.LastApplied != nil {
			lastApplied = meta.LastApplied.Format("2006-01-02 15:04")
		}
		fmt.Printf("  %-20s %-10s %-8s %-20s %s\n",
			meta.Name, meta.Source, meta.Version, meta.SHA256[:16]+"…", lastApplied)
	}
	fmt.Printf("\n  Total: %d profiles\n", len(profiles))
	fmt.Println()
	return nil
}

func runProfileShow(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := manifest.SanitizeProfileName(name); err != nil {
		return err
	}

	store, err := initProfileStore()
	if err != nil {
		return err
	}

	// Load with extends resolution
	profile, resolveErr := store.LoadProfileWithExtends(name)
	meta, hasMeta := store.GetMeta(name)

	if outputJSON {
		result := map[string]interface{}{
			"profile": profile,
			"meta":    meta,
			"valid":   resolveErr == nil,
		}
		if resolveErr != nil {
			result["error"] = resolveErr.Error()
		}
		return jsonOutput(result)
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Printf("  ║   NEXUS PROFILE: %-30s ║\n", name)
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	if hasMeta {
		fmt.Printf("  Source:        %s\n", meta.Source)
		fmt.Printf("  Version:       %s\n", meta.Version)
		fmt.Printf("  SHA256:        %s\n", meta.SHA256)
		if meta.LastApplied != nil {
			fmt.Printf("  Last Applied:  %s\n", meta.LastApplied.Format("2006-01-02 15:04:05"))
		}
		fmt.Println()
	}

	if resolveErr != nil {
		fmt.Printf("  ⚠️  Profile resolution warning: %v\n", resolveErr)
		fmt.Println()
	}

	if profile != nil {
		fmt.Printf("  Name:          %s\n", profile.Name)
		fmt.Printf("  Version:       %s\n", profile.Version)
		fmt.Printf("  Description:   %s\n", profile.Description)
		fmt.Printf("  Author:        %s\n", profile.Author)
		if profile.Extends != "" {
			fmt.Printf("  Extends:       %s\n", profile.Extends)
		}

		fmt.Println()
		fmt.Println("  ── TARGETS ────────────────────────────────────")
		for _, target := range profile.Targets {
			fmt.Printf("  📦 %s: %v\n", target.Family, target.Packages)
		}

		if len(profile.Env) > 0 {
			fmt.Println()
			fmt.Println("  ── ENVIRONMENT VARIABLES ─────────────────────")
			for k, v := range profile.Env {
				fmt.Printf("  🔑 %s=%s\n", k, v)
			}
		}
	} else {
		// Show raw content
		content, err := store.ProfileContent(name)
		if err != nil {
			return fmt.Errorf("could not read profile: %w", err)
		}
		fmt.Println("  ── RAW CONTENT ──────────────────────────────")
		fmt.Println(content)
	}

	fmt.Println()
	if resolveErr != nil {
		return fmt.Errorf("profile resolution failed: %w", resolveErr)
	}
	return nil
}

func runProfileValidate(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	ctx := context.Background()
	rdeps, err := initRunnerDeps(ctx)
	if err != nil {
		return err
	}

	data, readErr := os.ReadFile(filePath) //nolint:gosec
	if readErr != nil {
		return fmt.Errorf("failed to read file: %w", readErr)
	}

	profile, validateErr := rdeps.ValidateProfileBytes(data)
	if validateErr != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"valid": false, "error": validateErr.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "  ⛔ INVALID: %v\n", validateErr)
		}
		return fmt.Errorf("invalid profile: %w", validateErr)
	}

	if outputJSON {
		return jsonOutput(map[string]interface{}{
			"valid": true, "name": profile.Name, "version": profile.Version,
		})
	}

	fmt.Println()
	fmt.Println("  ✅ VALID — Profile passes all schema and semantic checks")
	fmt.Printf("  Name:    %s\n", profile.Name)
	fmt.Printf("  Version: %s\n", profile.Version)
	fmt.Printf("  Targets: %d families\n", len(profile.Targets))
	if profile.Extends != "" {
		fmt.Printf("  Extends: %s\n", profile.Extends)
	}
	fmt.Println()
	return nil
}

func runProfileCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := manifest.SanitizeProfileName(name); err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║       NEXUS PROFILE CREATOR — Interactive        ║")
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Printf("  Profile name: %s\n", name)

	fmt.Print("  Description: ")
	description, _ := reader.ReadString('\n')
	description = strings.TrimSpace(description)

	fmt.Print("  Author: ")
	author, _ := reader.ReadString('\n')
	author = strings.TrimSpace(author)

	fmt.Print("  Extends (profile name, or empty): ")
	extends, _ := reader.ReadString('\n')
	extends = strings.TrimSpace(extends)

	fmt.Print("  Target families (comma-separated, e.g. debian,arch,fedora,alpine): ")
	familiesInput, _ := reader.ReadString('\n')
	familiesInput = strings.TrimSpace(familiesInput)
	families := strings.Split(familiesInput, ",")

	profile := &manifest.NexusProfile{
		Name:        name,
		Version:     "1.0.0",
		Description: description,
		Author:      author,
		Extends:     extends,
		Targets:     []manifest.TargetConfig{},
		Env:         map[string]string{},
	}

	for _, f := range families {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !manifest.AllowedPackageFamilies[f] {
			fmt.Printf("  ⚠️  Skipping unknown family: %s\n", f)
			continue
		}
		fmt.Printf("  Packages for %s (comma-separated): ", f)
		pkgsInput, _ := reader.ReadString('\n')
		pkgsInput = strings.TrimSpace(pkgsInput)
		var pkgs []string
		for _, p := range strings.Split(pkgsInput, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				pkgs = append(pkgs, p)
			}
		}
		if len(pkgs) > 0 {
			profile.Targets = append(profile.Targets, manifest.TargetConfig{
				Family:   f,
				Packages: pkgs,
			})
		}
	}

	fmt.Print("  Env vars (KEY=VALUE, comma-separated, or empty): ")
	envInput, _ := reader.ReadString('\n')
	envInput = strings.TrimSpace(envInput)
	if envInput != "" {
		for _, pair := range strings.Split(envInput, ",") {
			parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(parts) == 2 {
				profile.Env[parts[0]] = parts[1]
			}
		}
	}
	profile.Env["NEXUS_PROFILE"] = name

	// Validate
	if err := manifest.Validate(profile); err != nil {
		fmt.Fprintf(os.Stderr, "  ⛔ Validation failed: %v\n", err)
		return fmt.Errorf("validation failed: %w", err)
	}

	// Serialize
	yamlContent, err := manifest.FormatProfileYAML(profile)
	if err != nil {
		return fmt.Errorf("failed to serialize profile: %w", err)
	}

	// Schema validate
	if _, err := manifest.ParseBytes([]byte(yamlContent)); err != nil {
		fmt.Fprintf(os.Stderr, "  ⛔ Schema validation failed: %v\n", err)
		return fmt.Errorf("schema validation failed: %w", err)
	}

	// Save to store
	store, storeErr := initProfileStore()
	if storeErr != nil {
		return storeErr
	}

	if err := store.SaveProfile(name, []byte(yamlContent), manifest.SourceLocal); err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("  ✅ Profile '%s' created and saved to profile store\n", name)
	fmt.Printf("  📁 Path: %s\n", store.ProfilePath(name))
	fmt.Println()
	return nil
}

func runProfileFetch(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := manifest.SanitizeProfileName(name); err != nil {
		return err
	}

	store, err := initProfileStore()
	if err != nil {
		return err
	}

	remoteBase := manifest.DefaultRemoteBaseURL()

	if !outputJSON {
		fmt.Println()
		fmt.Printf("  🌐 Fetching profile '%s' from %s/...\n", name, remoteBase)
	}

	if err := store.FetchProfile(name, remoteBase); err != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"success": false, "error": err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "  ⛔ Fetch failed: %v\n", err)
		}
		return fmt.Errorf("fetch failed: %w", err)
	}

	meta, _ := store.GetMeta(name)

	if outputJSON {
		return jsonOutput(map[string]interface{}{"success": true, "name": name, "meta": meta})
	}

	fmt.Printf("  ✅ Profile '%s' fetched and validated\n", name)
	fmt.Printf("  🔒 SHA256: %s\n", meta.SHA256[:32]+"…")
	fmt.Println()
	return nil
}

func runProfileRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := manifest.SanitizeProfileName(name); err != nil {
		return err
	}

	store, err := initProfileStore()
	if err != nil {
		return err
	}

	if err := store.RemoveProfile(name, forceRemove); err != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"success": false, "error": err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "  ⛔ %v\n", err)
		}
		return fmt.Errorf("remove failed: %w", err)
	}

	if outputJSON {
		return jsonOutput(map[string]interface{}{"success": true, "removed": name})
	}

	fmt.Printf("  ✅ Profile '%s' removed from store\n", name)
	return nil
}

func runProfileVerify(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := manifest.SanitizeProfileName(name); err != nil {
		return err
	}

	store, err := initProfileStore()
	if err != nil {
		return err
	}

	err = store.VerifyIntegrity(name)
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"valid": false, "error": err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "  ⛔ INTEGRITY CHECK FAILED: %v\n", err)
		}
		return fmt.Errorf("integrity check failed: %w", err)
	}

	meta, _ := store.GetMeta(name)
	if outputJSON {
		return jsonOutput(map[string]interface{}{"valid": true, "name": name, "sha256": meta.SHA256})
	}

	fmt.Printf("  ✅ Profile '%s' integrity verified (SHA256: %s…)\n", name, meta.SHA256[:32])
	return nil
}

func runProfileSuggest(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	info, err := engine.Probe(ctx)
	if err != nil {
		// Non-fatal — partial data is still useful
	}

	// Load bundled defaults to suggest from
	profiles := make([]manifest.NexusProfile, 0)
	for name, content := range manifest.BundledDefaults() {
		parsed, pErr := manifest.ParseBytes([]byte(content))
		if pErr != nil {
			continue
		}
		parsed.Name = name
		profiles = append(profiles, *parsed)
	}

	suggestions := engine.SuggestProfile(info, profiles)

	if outputJSON {
		return jsonOutput(map[string]interface{}{
			"system":      info,
			"suggestions": suggestions,
		})
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║   NEXUS PROTOCOL — PROFILE SUGGESTIONS          ║")
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	if info.DistroID != "" {
		fmt.Printf("  Detected: %s %s\n", info.DistroID, info.DistroVersion)
	} else {
		fmt.Printf("  Detected: %s/%s\n", info.OS, info.Arch)
	}
	if info.GPU != "" {
		fmt.Printf("  GPU:      %s\n", info.GPU)
	}
	fmt.Printf("  RAM:      %d MB\n", info.RAMTotalMB)
	if info.IsWSL2 {
		fmt.Println("  WSL2:     Yes")
	}
	fmt.Println()

	fmt.Print(engine.FormatProfileSuggestions(suggestions))

	fmt.Println("  Recommendations:")
	for _, s := range suggestions[:min(len(suggestions), 3)] {
		fmt.Printf("    • %s — %s\n", s.Name, s.Compatibility.Message)
	}
	fmt.Println()
	fmt.Println("  Run 'nexus profile apply <name>' to install a profile.")
	fmt.Println()
	return nil
}

func runProfileApply(cmd *cobra.Command, args []string) error {
	name := args[0]
	if err := manifest.SanitizeProfileName(name); err != nil {
		return err
	}

	ctx := context.Background()
	rdeps, err := initRunnerDeps(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = rdeps.Audit.Close() }()
	rdeps.DryRun = dryRun

	if !outputJSON {
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════════════════╗")
		fmt.Printf("  ║   NEXUS PROFILE APPLY: %-24s ║\n", name)
		fmt.Println("  ╚══════════════════════════════════════════════════╝")
		fmt.Println()
	}

	result, profile, orchErr := rdeps.ApplyProfile(ctx, name)

	if outputJSON {
		output := map[string]interface{}{
			"profile": name,
			"result":  result,
		}
		if profile != nil {
			output["target"] = manifest.ResolveTarget(profile, rdeps.Env.PackageManager)
		}
		if orchErr != nil {
			output["error"] = orchErr.Error()
		}
		return jsonOutput(output)
	}

	if profile != nil {
		fmt.Printf("  📄 Profile:       %s v%s\n", profile.Name, profile.Version)
		fmt.Printf("  📝 Description:   %s\n", profile.Description)
		if profile.Extends != "" {
			fmt.Printf("  🧬 Extends:       %s\n", profile.Extends)
		}
		target := manifest.ResolveTarget(profile, rdeps.Env.PackageManager)
		if target != nil {
			fmt.Printf("  📦 Target:        %s (%d packages)\n", target.Family, len(target.Packages))
		}
		fmt.Printf("  🔧 Package Manager: %s\n", rdeps.PM.Name())
		if dryRun {
			fmt.Printf("  🔄 Mode:          DRY RUN\n")
		}
		fmt.Println()
	}

	if orchErr != nil {
		fmt.Fprintf(os.Stderr, "  ⛔ %v\n", orchErr)
		return fmt.Errorf("apply failed: %w", orchErr)
	}

	fmt.Println(installer.FormatOrchestratorResult(result))

	// Apply profile env vars to shell config on success
	if !dryRun && result != nil && !result.Aborted && profile != nil {
		envVars := make(map[string]string)
		for k, v := range profile.Env {
			envVars[k] = v
		}
		envVars["NEXUS_PROFILE"] = name
		configureResult := engine.Configure(ctx, envVars)
		for _, msg := range configureResult.Messages {
			fmt.Printf("    %s\n", msg)
		}

		fmt.Printf("  ✅ Profile '%s' applied successfully\n", name)
	} else if dryRun {
		fmt.Printf("  🔄 [DRY RUN] Profile '%s' — no changes made\n", name)
	}

	fmt.Println()
	return nil
}

func jsonOutput(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

// ─── V4 Commands: WSL2 Detection ───

func runWSLStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	status := bridge.DetectWSL2Status(ctx)

	if outputJSON {
		return jsonOutput(status)
	}

	fmt.Print(bridge.FormatWSL2Status(status))
	return nil
}

func runWSLCheck(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	status := bridge.DetectWSL2Status(ctx)

	if outputJSON {
		// Per V4 docs: "Quick readiness check — exits 0 if ready, 1 if not.
		// Useful for scripting and CI/CD pipelines."
		// The JSON path MUST also exit 1 when not ready, so that
		// CI/CD pipelines can reliably test readiness via exit code.
		if err := jsonOutput(map[string]interface{}{
			"ready":    status.Ready,
			"blockers": status.Blockers,
		}); err != nil {
			return err
		}
		if !status.Ready {
			return fmt.Errorf("system is not ready for WSL2 setup")
		}
		return nil
	}

	fmt.Print(bridge.FormatWSL2Check(status))

	if !status.Ready {
		return fmt.Errorf("system is not ready for WSL2 setup")
	}
	return nil
}

// ─── V5 Commands: WSL2 Import (The Bridge) ───

// runWSLImport handles `nexus wsl import [image]`
func runWSLImport(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if !wsl.IsImportAvailable() {
		errMsg := "WSL2 import is only available on Windows. On Linux, use 'nexus init' instead"
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": errMsg, "available": false})
		} else {
			fmt.Printf("\n  ⛔ %s\n\n", errMsg)
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Determine image name (default: nexus-alpine)
	imageName := "nexus-alpine"
	if len(args) > 0 {
		imageName = args[0]
	}

	// Find image in registry
	image, err := wsl.FindImage(imageName)
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": err.Error(), "available_images": wsl.DefaultRootFSRegistry()})
		} else {
			fmt.Printf("\n  ⛔ %v\n\n", err)
		}
		return fmt.Errorf("failed to find image %s: %w", imageName, err)
	}

	// Validate distro name
	if err := wsl.ValidateDistroName(wslDistroName); err != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": fmt.Sprintf("invalid distro name: %v", err)})
		} else {
			fmt.Printf("\n  ⛔ Invalid distro name: %v\n\n", err)
		}
		return fmt.Errorf("invalid distro name: %w", err)
	}

	// Determine install path
	homeDir, _ := os.UserHomeDir()
	installPath := homeDir + "/.nexus/wsl/" + wslDistroName

	// Validate install path
	if err := wsl.ValidateInstallPath(installPath); err != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": fmt.Sprintf("invalid install path: %v", err)})
		} else {
			fmt.Printf("\n  ⛔ Invalid install path: %v\n\n", err)
		}
		return fmt.Errorf("invalid install path: %w", err)
	}

	// Create the importer
	wslImporter := wsl.NewWSL2Importer(wsl.ExecFunc(engine.SanitizeAndExecute))

	// Set up state recording callback
	state, stateErr := engine.NewStateTracker()
	if stateErr == nil {
		wslImporter.SetStateRecordFunc(state.RecordWSLImport)
	}

	// Set up audit callback
	audit, auditErr := engine.NewAuditLogger()
	if auditErr == nil {
		wslImporter.SetAuditFunc(func(action, target, result string, durationMs int64, err error) {
			entry := engine.AuditEntry{Action: action, Target: target, Result: result, DurationMs: durationMs}
			if err != nil {
				entry.Error = err.Error()
			}
			_ = audit.Log(entry)
		})
		defer func() { _ = audit.Close() }()
	}

	// Build import config
	importCfg := &wsl.ImportConfig{
		DistroName:   wslDistroName,
		InstallPath:  installPath,
		Image:        image,
		SkipDownload: wslSkipDownload,
		SkipVerify:   wslSkipVerify,
		DryRun:       dryRun,
	}

	if !outputJSON {
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════════════════╗")
		fmt.Println("  ║   NEXUS PROTOCOL — WSL2 IMPORT (The Bridge)     ║")
		fmt.Println("  ╚══════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Printf("  📦 Image:         %s v%s\n", image.Name, image.Version)
		fmt.Printf("  🐧 Distribution:  %s\n", wslDistroName)
		fmt.Printf("  📁 Install Path:  %s\n", installPath)
		fmt.Printf("  🏗️  Architecture:  %s\n", image.Arch)
		if dryRun {
			fmt.Println("  🔄 [DRY RUN] No changes will be made")
		}
		fmt.Println()
	}

	result, importErr := wslImporter.Import(ctx, importCfg)

	// State recording is handled by the StateRecordFunc callback inside Import().
	// No duplicate recording here — the callback is invoked in Step 7 (record).

	if outputJSON {
		return jsonOutput(result)
	}

	if importErr != nil {
		fmt.Fprintf(os.Stderr, "  ⛔ Import failed: %v\n", importErr)
	}

	fmt.Print(wsl.FormatImportResult(result))
	return nil
}

// runWSLSetup handles `nexus wsl setup` — the 60-second promise
func runWSLSetup(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if !wsl.IsImportAvailable() {
		errMsg := "WSL2 import is only available on Windows. On Linux, use 'nexus init' instead"
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": errMsg, "available": false})
		} else {
			fmt.Printf("\n  ⛔ %s\n\n", errMsg)
		}
		return fmt.Errorf("%s", errMsg)
	}

	if !outputJSON {
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════════════════╗")
		fmt.Println("  ║   NEXUS PROTOCOL — ONE-COMMAND WSL2 SETUP       ║")
		fmt.Println("  ║              The 60-Second Promise               ║")
		fmt.Println("  ╚══════════════════════════════════════════════════╝")
		fmt.Println()
	}

	// Step 1: Check WSL2 readiness
	status := bridge.DetectWSL2Status(ctx)
	if !status.Ready {
		if outputJSON {
			return jsonOutput(map[string]interface{}{
				"ready": false, "blockers": status.Blockers,
				"recommendations": status.Recommendations,
			})
		} else {
			fmt.Println("  ⛔ System is NOT ready for WSL2 setup")
			fmt.Print(bridge.FormatWSL2Check(status))
		}
		return fmt.Errorf("system is not ready for WSL2 setup")
	}

	if !outputJSON {
		fmt.Println("  ✅ WSL2 readiness confirmed")
	}

	// Step 2: Import with default Alpine image
	image, err := wsl.FindImage("nexus-alpine")
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("  ⛔ %v\n", err)
		}
		return fmt.Errorf("failed to find default image: %w", err)
	}

	if err := wsl.ValidateDistroName(wslDistroName); err != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": fmt.Sprintf("invalid distro name: %v", err)})
		} else {
			fmt.Printf("  ⛔ Invalid distro name: %v\n", err)
		}
		return fmt.Errorf("invalid distro name: %w", err)
	}

	homeDir, _ := os.UserHomeDir()
	installPath := homeDir + "/.nexus/wsl/" + wslDistroName

	wslImporter := wsl.NewWSL2Importer(wsl.ExecFunc(engine.SanitizeAndExecute))

	state, _ := engine.NewStateTracker()
	if state != nil {
		wslImporter.SetStateRecordFunc(state.RecordWSLImport)
	}
	audit, _ := engine.NewAuditLogger()
	if audit != nil {
		wslImporter.SetAuditFunc(func(action, target, result string, durationMs int64, err error) {
			entry := engine.AuditEntry{Action: action, Target: target, Result: result, DurationMs: durationMs}
			if err != nil {
				entry.Error = err.Error()
			}
			_ = audit.Log(entry)
		})
		defer func() { _ = audit.Close() }()
	}

	importCfg := &wsl.ImportConfig{
		DistroName:  wslDistroName,
		InstallPath: installPath,
		Image:       image,
		DryRun:      dryRun,
	}

	if !outputJSON {
		fmt.Printf("  📦 Importing %s as '%s'...\n\n", image.Name, wslDistroName)
	}

	result, importErr := wslImporter.Import(ctx, importCfg)

	// State recording is handled by the StateRecordFunc callback inside Import().
	// No duplicate recording here — the callback is invoked in Step 7 (record).

	if outputJSON {
		return jsonOutput(result)
	}

	if importErr != nil {
		fmt.Fprintf(os.Stderr, "  ⛔ Setup failed: %v\n", importErr)
	}

	fmt.Print(wsl.FormatImportResult(result))

	if result != nil && !result.Aborted {
		fmt.Println("  ╔══════════════════════════════════════════════════╗")
		fmt.Println("  ║   ✅ NEXUS WSL2 SETUP COMPLETE                   ║")
		fmt.Printf("  ║   Run: wsl -d %s\n", wslDistroName)
		fmt.Println("  ╚══════════════════════════════════════════════════╝")
		fmt.Println()
	}

	return nil
}

// runWSLRemove handles `nexus wsl remove [distro]`
func runWSLRemove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	distroName := args[0]

	if !wsl.IsImportAvailable() {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": "WSL2 management is only available on Windows"})
		} else {
			fmt.Println("\n  ⛔ WSL2 management is only available on Windows")
		}
		return fmt.Errorf("WSL2 management is only available on Windows")
	}

	state, _ := engine.NewStateTracker()
	isManaged := false
	if state != nil {
		isManaged = state.IsWSLManaged(distroName)
	}

	if !isManaged && !forceRemove {
		errMsg := fmt.Sprintf("'%s' is not a Nexus-managed WSL2 distribution. Use --force to remove anyway.", distroName)
		if outputJSON {
			return jsonOutput(map[string]interface{}{
				"error":   errMsg,
				"managed": false,
			})
		} else {
			fmt.Printf("\n  ⚠️  '%s' is not a Nexus-managed WSL2 distribution.\n", distroName)
			fmt.Println("  Use --force to remove anyway.")
			fmt.Println()
		}
		return fmt.Errorf("%s", errMsg)
	}

	wslImporter := wsl.NewWSL2Importer(wsl.ExecFunc(engine.SanitizeAndExecute))

	// Set up audit callback for remove operation
	audit, _ := engine.NewAuditLogger()
	if audit != nil {
		wslImporter.SetAuditFunc(func(action, target, result string, durationMs int64, err error) {
			entry := engine.AuditEntry{Action: action, Target: target, Result: result, DurationMs: durationMs}
			if err != nil {
				entry.Error = err.Error()
			}
			_ = audit.Log(entry)
		})
		defer func() { _ = audit.Close() }()
	}

	if err := wslImporter.Remove(ctx, distroName, forceRemove); err != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("\n  ⛔ Failed to remove '%s': %v\n\n", distroName, err)
		}
		return fmt.Errorf("failed to remove distro %s: %w", distroName, err)
	}

	if state != nil {
		_ = state.RecordWSLRemove(distroName)
	}

	if outputJSON {
		return jsonOutput(map[string]interface{}{"removed": distroName, "status": "success"})
	}

	fmt.Printf("\n  ✅ Removed WSL2 distribution '%s'\n\n", distroName)
	return nil
}

// runWSLList handles `nexus wsl list`
func runWSLList(cmd *cobra.Command, args []string) error {
	state, _ := engine.NewStateTracker()
	instances := make(map[string]engine.WSLInstanceState)
	if state != nil {
		instances = state.GetWSLInstances()
	}

	if outputJSON {
		return jsonOutput(instances)
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║   NEXUS PROTOCOL — WSL2 INSTANCES               ║")
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	if len(instances) == 0 {
		fmt.Println("  No Nexus-managed WSL2 instances found.")
		fmt.Println("  Run 'nexus wsl setup' to create one.")
		fmt.Println()
		return nil
	}

	fmt.Printf("  %-15s %-15s %-10s %-20s %s\n", "NAME", "IMAGE", "FAMILY", "VERSION", "IMPORTED")
	fmt.Println("  " + strings.Repeat("─", 70))
	for name, inst := range instances {
		fmt.Printf("  %-15s %-15s %-10s %-20s %s\n",
			name, inst.ImageName, inst.Family, inst.ImageVersion,
			inst.ImportedAt.Format("2006-01-02 15:04"))
	}
	fmt.Printf("\n  Total: %d instances\n", len(instances))
	fmt.Println()
	return nil
}

// runWSLEnter handles `nexus wsl enter [distro]`
//
// Per V5 "The Bridge": After import, the user needs a simple way to enter
// their new Linux environment. This command delegates to `wsl -d <name>`
// which launches an interactive shell inside the distribution.
//
// SECURITY: The distro name is validated through SanitizeAndExecute.
// The command does not use shell metacharacters — it simply passes
// the validated distro name as an argument to `wsl -d`.
func runWSLEnter(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if !wsl.IsImportAvailable() {
		errMsg := "WSL2 is only available on Windows. On Linux, you're already running natively"
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": errMsg, "available": false})
		} else {
			fmt.Printf("\n  ⛔ %s\n\n", errMsg)
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Allow distro name from positional arg or --name flag
	distroName := wslDistroName
	if len(args) > 0 {
		distroName = args[0]
	}

	if err := wsl.ValidateDistroName(distroName); err != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": fmt.Sprintf("invalid distro name: %v", err)})
		} else {
			fmt.Printf("\n  ⛔ Invalid distro name: %v\n\n", err)
		}
		return fmt.Errorf("invalid distro name: %w", err)
	}

	// Check if the distro is Nexus-managed
	state, _ := engine.NewStateTracker()
	if state != nil && !state.IsWSLManaged(distroName) {
		if !outputJSON {
			fmt.Printf("  ⚠️  '%s' is not a Nexus-managed distribution. Entering anyway...\n", distroName)
		}
	}

	// Execute: wsl -d <name> — this launches an interactive shell
	// We use execFn directly instead of exec.Command because the Zero-Trust
	// architecture requires all commands to go through SanitizeAndExecute.
	// However, wsl -d launches an interactive TUI, so we need to use
	// the raw exec.Command approach to attach stdin/stdout/stderr.
	//
	// SECURITY NOTE: We validate the distro name above, and the command
	// structure is fixed (wsl -d <validated-name>). No shell injection possible.
	if !outputJSON {
		fmt.Printf("  🐧 Entering WSL2 distribution '%s'...\n\n", distroName)
	}

	output, err := engine.SanitizeAndExecute(ctx, "wsl", "-d", distroName)
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]interface{}{"error": err.Error()})
		} else {
			fmt.Printf("\n  ⛔ Failed to enter distribution '%s': %v\n\n", distroName, err)
		}
		return fmt.Errorf("failed to enter distribution %s: %w", distroName, err)
	}

	// If we got output (non-interactive mode), show it
	if output != "" && !outputJSON {
		fmt.Print(output)
	}

	return nil
}

// runWSLImages handles `nexus wsl images`
func runWSLImages(cmd *cobra.Command, args []string) error {
	images := wsl.DefaultRootFSRegistry()

	if outputJSON {
		return jsonOutput(images)
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║   NEXUS PROTOCOL — ROOTFS IMAGE REGISTRY        ║")
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	if len(images) == 0 {
		fmt.Println("  No images available in the registry.")
		fmt.Println()
		return nil
	}

	fmt.Printf("  %-18s %-10s %-8s %-8s %s\n", "NAME", "VERSION", "ARCH", "SIZE", "DESCRIPTION")
	fmt.Println("  " + strings.Repeat("─", 80))
	for _, img := range images {
		sizeStr := fmt.Sprintf("~%dMB", img.Size/(1024*1024))
		if img.Size < 1024*1024 {
			sizeStr = fmt.Sprintf("~%dKB", img.Size/1024)
		}
		fmt.Printf("  %-18s %-10s %-8s %-8s %s\n",
			img.Name, img.Version, img.Arch, sizeStr, img.Description)
	}
	fmt.Println()
	fmt.Println("  Usage: nexus wsl import <image-name>")
	fmt.Println("  Example: nexus wsl import nexus-alpine")
	fmt.Println()
	return nil
}

// ─── V7 Commands: Dotfiles Management ───

func runDotfilesDetect(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	report, err := dotfiles.Detect(ctx, nd.execFn)
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(report)
	}
	fmt.Printf("Chezmoi: installed=%v version=%s\n", report.Installed, report.Version)
	return nil
}

func runDotfilesInstall(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.InstallDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.InstallChezmoi(ctx, deps)
	return err
}

func runDotfilesInit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.SourceDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	return dotfiles.BindSource(ctx, args[0], deps)
}

func runDotfilesRemove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.SourceDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	return dotfiles.UnbindSource(ctx, deps)
}

func runDotfilesApply(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.ApplyDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.Apply(ctx, deps)
	return err
}

func runDotfilesStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.ApplyDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	status, err := dotfiles.Status(ctx, deps)
	if err != nil {
		return err
	}
	fmt.Println(status)
	return nil
}

func runDotfilesDiff(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.ApplyDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	diff, err := dotfiles.Diff(ctx, deps)
	if err != nil {
		return err
	}
	fmt.Println(diff)
	return nil
}

// ─── Missing helpers and corrected run function signatures ───

// shortSHA returns a short (7-char) prefix of a SHA for display.
// Empty input returns "<unknown>". Strings <= 7 chars are returned as-is.
// Test: TestShortSHA in main_test.go.
func shortSHA(s string) string {
	if s == "" {
		return "<unknown>"
	}
	if len(s) <= 7 {
		return s
	}
	return s[:7]
}

// resolveSyncToken returns the GitHub PAT to use for dotfile sync.
// Priority: explicit flag > NEXUS_DOTFILES_TOKEN env var > "".
// Tests: TestResolveSyncToken_* in main_test.go.
func resolveSyncToken(flagToken string) string {
	if flagToken != "" {
		return flagToken
	}
	return os.Getenv("NEXUS_DOTFILES_TOKEN")
}

// reportMatchesForJSON converts a slice of secret-scan matches into a
// JSON-friendly value. Returns nil for nil input so the caller can
// embed the result directly in a JSON response and have it serialize
// as `null` rather than `[]`.
// Test: TestReportMatchesForJSON in main_test.go.
func reportMatchesForJSON(matches []dotfiles.Match) interface{} {
	if matches == nil {
		return nil
	}
	return matches
}

// ─── Corrected run function signatures (matching test expectations) ───

// runDotfilesAdd adds a file to managed dotfiles.
// Test signature: runDotfilesAdd(cmd, args, force bool).
func runDotfilesAdd(cmd *cobra.Command, args []string, force bool) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.AddDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.Add(ctx, args[0], deps, force)
	return err
}

// runDotfilesPush pushes local changes to remote.
// Test signature: runDotfilesPush(cmd, message string, force bool, token string).
func runDotfilesPush(cmd *cobra.Command, message string, force bool, token string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.SyncDeps{
		ExecFn:         nd.execFn,
		State:          nd.state,
		Audit:          nd.audit,
		Token:          resolveSyncToken(token),
		SkipSecretScan: force,
	}
	_, err = dotfiles.Push(ctx, deps, message)
	return err
}

// runDotfilesPull pulls from remote.
// Test signature: runDotfilesPull(cmd, rebase bool, token string).
func runDotfilesPull(cmd *cobra.Command, rebase bool, token string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.SyncDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
		Token:  resolveSyncToken(token),
	}
	_, err = dotfiles.Pull(ctx, deps, rebase)
	return err
}

// runDotfilesSync does pull + apply + push.
// Test signature: runDotfilesSync(cmd, message string, force bool, token string).
func runDotfilesSync(cmd *cobra.Command, message string, force bool, token string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.SyncDeps{
		ExecFn:         nd.execFn,
		State:          nd.state,
		Audit:          nd.audit,
		Token:          resolveSyncToken(token),
		SkipSecretScan: force,
	}
	_, err = dotfiles.Sync(ctx, deps, message, false)
	return err
}

// runDotfilesVaultAdd encrypts a file with age.
// Test signature: runDotfilesVaultAdd(cmd, file string, force bool).
func runDotfilesVaultAdd(cmd *cobra.Command, file string, force bool) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.VaultAddDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
		DryRun: force,
	}
	_, err = dotfiles.VaultAdd(ctx, file, deps)
	return err
}

// ─── Re-adding run functions that were deleted by sed ───

func runDotfilesVerify(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.AddDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.Verify(ctx, deps)
	return err
}

func runDotfilesVaultInit(cmd *cobra.Command, force bool) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.VaultInitDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.VaultInit(ctx, deps)
	return err
}

func runDotfilesVaultList(cmd *cobra.Command) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	result, err := dotfiles.VaultList(struct {
		State *engine.StateTracker
	}{State: nd.state})
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(result)
	}
	for _, entry := range result.Files {
		fmt.Printf("%s -> %s\n", entry.Original, entry.Encrypted)
	}
	return nil
}

func runDotfilesVaultStatus(cmd *cobra.Command) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	result, err := dotfiles.VaultStatus(struct {
		State *engine.StateTracker
	}{State: nd.state})
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(result)
	}
	fmt.Printf("Vault status: initialized=%v files=%d\n", result.Status.Initialized, result.Status.FileCount)
	return nil
}

func runDotfilesVaultUnlock(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.VaultUnlockDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.VaultUnlock(ctx, deps)
	return err
}

func runDotfilesVaultRemove(cmd *cobra.Command, file string, force bool) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.VaultRemoveDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
		Force:  force,
	}
	_, err = dotfiles.VaultRemove(ctx, file, deps)
	return err
}

// ─── Mode run functions ───

func runModeList(cmd *cobra.Command, args []string) error {
	_, err := initDeps(context.Background())
	if err != nil {
		return err
	}
	modes, err := mode.List()
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(modes)
	}
	for _, m := range modes {
		builtin := ""
		if m.Builtin {
			builtin = " (built-in)"
		}
		fmt.Printf("%s — %s%s\n", m.Name, m.Description, builtin)
	}
	return nil
}

func runModeCurrent(cmd *cobra.Command, args []string) error {
	nd, err := initDeps(context.Background())
	if err != nil {
		return err
	}
	active := nd.state.GetActiveMode()
	if outputJSON {
		return jsonOutput(map[string]string{"active_mode": active})
	}
	if active == "" {
		fmt.Println("No mode has been applied yet.")
	} else {
		fmt.Printf("Active mode: %s\n", active)
	}
	return nil
}

func runModeShow(cmd *cobra.Command, args []string) error {
	m, err := mode.Resolve(args[0])
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(m)
	}
	fmt.Printf("Mode: %s (%s)\n", m.Name, m.Description)
	return nil
}

func runModeApply(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}

	deps := mode.ApplyDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
		GOOS:   runtime.GOOS,
		ApplyProfile: func(ctx context.Context, pName string, dryRun bool) error {
			store, storeErr := initProfileStore()
			if storeErr != nil {
				return storeErr
			}
			profile, profileErr := store.LoadProfileWithExtends(pName)
			if profileErr != nil {
				return profileErr
			}
			target := manifest.ResolveTarget(profile, nd.env.PackageManager)
			if target == nil {
				return nil
			}
			orch := installer.NewOrchestrator(nd.pm, nd.execFn, nd.state, nd.audit, pName, dryRun)
			_, installErr := orch.Install(ctx, target.Packages)
			return installErr
		},
		BindDotfiles: func(ctx context.Context, source string) error { return nil },
	}

	opts := mode.ApplyOpts{
		DryRun:                dryRun,
		AllowUnlistedServices: allowUnlistedServices,
	}

	report, applyErr := mode.Apply(ctx, deps, args[0], opts)
	if applyErr != nil {
		return applyErr
	}

	if outputJSON {
		return jsonOutput(report)
	}
	fmt.Printf("Mode %q applied (previous: %q)\n", report.Mode, report.Previous)
	return nil
}

func runModeRollback(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := mode.ApplyDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
		GOOS:   runtime.GOOS,
	}
	report, err := mode.Rollback(ctx, deps, mode.ApplyOpts{})
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(report)
	}
	fmt.Printf("Rolled back to: %s\n", report.Mode)
	return nil
}

func runModeDefine(cmd *cobra.Command, args []string) error {
	input := mode.DefineInput{
		In:             bufio.NewReader(os.Stdin),
		Out:            os.Stdout,
		NonInteractive: false,
		Draft:          mode.Mode{Name: args[0]},
	}
	m, defineErr := mode.Define(input)
	if defineErr != nil {
		return defineErr
	}
	fmt.Printf("Mode %q created at %s\n", m.Name, m.SourcePath)
	return nil
}

// ─── Container run functions ───

func runContainerList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	list, err := container.List(ctx, nd.state, nd.execFn)
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(list)
	}
	fmt.Println("Containers:")
	for _, c := range list.Containers {
		managed := ""
		if c.Managed {
			managed = " (managed)"
		}
		fmt.Printf("  %s — %s%s\n", c.Name, c.Status, managed)
	}
	return nil
}

func runContainerInfo(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	info, err := container.Info(ctx, nd.state, nd.execFn, args[0])
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(info)
	}
	fmt.Printf("Name: %s\nStatus: %s\nImage: %s\nManaged: %v\n", info.Name, info.Status, info.Image, info.Managed)
	return nil
}

func runContainerCreate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	image, _ := cmd.Flags().GetString("image")
	deps := container.CreateDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	opts := container.CreateOpts{Image: image}
	report, err := container.Create(ctx, deps, args[0], opts)
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(report)
	}
	fmt.Printf("Container %q created\n", report.Name)
	return nil
}

func runContainerEnter(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	enterCmd, err := container.EnterCommand(args[0], nd.state)
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(map[string]string{"command": enterCmd})
	}
	fmt.Println("To enter this container, run:")
	fmt.Printf("  %s\n", enterCmd)
	return nil
}

func runContainerApps(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	apps, err := container.Apps(ctx, nd.state, nd.execFn, args[0])
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(apps)
	}
	fmt.Printf("Apps in %s:\n", apps.Name)
	for _, a := range apps.Apps {
		fmt.Printf("  %s\n", a)
	}
	return nil
}

func runContainerRemove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")
	opts := container.RemoveOpts{Force: force}
	report, err := container.Remove(ctx, nd.state, nd.execFn, args[0], opts)
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(report)
	}
	fmt.Printf("Removed: %s\n", report.Name)
	return nil
}

// ─── V13 Commands: Hardware Ledger ───

func runLedgerRecord(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	state, err := engine.NewStateTracker()
	if err != nil {
		return err
	}

	if !outputJSON {
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════════════════╗")
		fmt.Println("  ║   NEXUS PROTOCOL — HARDWARE LEDGER RECORD       ║")
		fmt.Println("  ╚══════════════════════════════════════════════════╝")
		fmt.Println()
	}

	if err := ledger.RecordSimple(ctx, state); err != nil {
		if outputJSON {
			return jsonOutput(map[string]string{"status": "error", "error": err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "  ⛔ Failed to record: %v\n", err)
		}
		return err
	}

	ledgerState := state.GetLedger()
	if outputJSON {
		return jsonOutput(map[string]interface{}{
			"status": "recorded",
			"count":  len(ledgerState.Records),
		})
	}

	fmt.Printf("  ✅ Hardware report recorded (%d total entries)\n", len(ledgerState.Records))
	fmt.Println()
	return nil
}

func runLedgerStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}

	ledgerState := nd.state.GetLedger()

	if outputJSON {
		return jsonOutput(ledgerState)
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║   NEXUS PROTOCOL — HARDWARE LEDGER STATUS       ║")
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	count := len(ledgerState.Records)
	fmt.Printf("  📊 Total records:    %d\n", count)

	if count > 0 {
		first := ledgerState.Records[0]
		last := ledgerState.Records[count-1]
		fmt.Printf("  🕐 First record:     %s\n", first.RecordedAt.Format("2006-01-02 15:04"))
		fmt.Printf("  🕐 Last record:      %s\n", last.RecordedAt.Format("2006-01-02 15:04"))

		successes := 0
		for _, r := range ledgerState.Records {
			if r.Success {
				successes++
			}
		}
		pct := float64(successes) / float64(count) * 100
		fmt.Printf("  ✅ Success rate:     %.0f%% (%d/%d)\n", pct, successes, count)
	}

	if !ledgerState.LastSyncedAt.IsZero() {
		fmt.Printf("  ☁️  Last synced:      %s\n", ledgerState.LastSyncedAt.Format("2006-01-02 15:04"))
	}
	if ledgerState.CommunitySyncEnabled {
		fmt.Println("  🌐 Community sync:  ENABLED")
	} else {
		fmt.Println("  🌐 Community sync:  DISABLED (run 'nexus ledger sync --enable' to opt in)")
	}

	fmt.Println()
	return nil
}

func runLedgerQuery(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}

	field, value := args[0], args[1]
	report, err := ledger.QueryField(ctx, nd.state, field, value)
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "  ⛔ %v\n", err)
		}
		return err
	}

	if outputJSON {
		return jsonOutput(report)
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Printf("  ║   LEDGER QUERY: %-24s ║\n", field+":"+value)
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Printf("  🔍 Field:           %s\n", report.Field)
	fmt.Printf("  🔍 Value:           %s\n", report.Value)
	fmt.Printf("  📊 Matching recs:   %d\n", report.Matches)
	fmt.Printf("  📊 Total records:   %d\n", report.TotalRecords)
	if report.Matches > 0 {
		fmt.Printf("  ✅ Success rate:    %.0f%%\n", report.SuccessRate*100)
	}

	fmt.Println()
	return nil
}

func runLedgerCheck(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}

	report, err := ledger.CheckHardware(ctx, nd.state)
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "  ⛔ %v\n", err)
		}
		return err
	}

	if outputJSON {
		return jsonOutput(report)
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║   NEXUS PROTOCOL — HARDWARE COMPATIBILITY CHECK ║")
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	if report.Unknown {
		fmt.Println("  ❓ Unknown hardware — no records found for this configuration.")
		fmt.Println("  Run 'nexus ledger record' to seed the ledger.")
	} else if report.HardwareOK {
		fmt.Printf("  ✅ This hardware is known to work (%.0f%% success rate over %d record(s))\n",
			report.SuccessRate*100, report.TotalRecords)
	} else {
		fmt.Printf("  ⚠️  This hardware has a low success rate (%.0f%% over %d record(s))\n",
			report.SuccessRate*100, report.TotalRecords)
	}

	for _, w := range report.Warnings {
		fmt.Printf("  ⚠️  %s\n", w)
	}

	fmt.Println()
	return nil
}

func runLedgerSync(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}

	enable, _ := cmd.Flags().GetBool("enable")
	disable, _ := cmd.Flags().GetBool("disable")

	if enable && disable {
		return fmt.Errorf("cannot set both --enable and --disable")
	}

	if enable {
		if err := nd.state.SetCommunitySyncEnabled(true); err != nil {
			return err
		}
		if !outputJSON {
			fmt.Println("  🌐 Community sync enabled")
		}
	}

	if disable {
		if err := nd.state.SetCommunitySyncEnabled(false); err != nil {
			return err
		}
		if !outputJSON {
			fmt.Println("  🌐 Community sync disabled")
		}
	}

	deps := ledger.SyncDeps{State: nd.state}
	if err := ledger.Sync(ctx, deps); err != nil {
		if outputJSON {
			return jsonOutput(map[string]string{"status": "error", "error": err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "  ⛔ %v\n", err)
		}
		return err
	}

	if outputJSON {
		return jsonOutput(map[string]string{"status": "synced"})
	}

	fmt.Println("  ✅ Ledger synced to community registry")
	fmt.Println()
	return nil
}

func runLedgerPull(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}

	deps := ledger.SyncDeps{State: nd.state}
	if err := ledger.Pull(ctx, deps); err != nil {
		if outputJSON {
			return jsonOutput(map[string]string{"status": "error", "error": err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "  ⛔ %v\n", err)
		}
		return err
	}

	if outputJSON {
		return jsonOutput(map[string]string{"status": "pulled"})
	}

	fmt.Println("  ✅ Community data pulled")
	fmt.Println()
	return nil
}

// ─── V14 Commands: Teleport Migration ───

func runTeleport(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if !outputJSON {
		fmt.Println()
		fmt.Println("  ╔══════════════════════════════════════════════════╗")
		fmt.Println("  ║   NEXUS PROTOCOL — TELEPORT MIGRATION TOOL      ║")
		fmt.Println("  ╚══════════════════════════════════════════════════╝")
		fmt.Println()
	}

	if dryRun && !outputJSON {
		fmt.Println("  🔍 DRY RUN — no changes will be made")
		fmt.Println()
	}

	results, err := engine.Teleport(ctx, dryRun)
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]string{"status": "error", "error": err.Error()})
		} else {
			fmt.Fprintf(os.Stderr, "  ⛔ %v\n", err)
		}
		return err
	}

	if outputJSON {
		return jsonOutput(map[string]interface{}{
			"status":  "ok",
			"results": results,
		})
	}

	fmt.Print(engine.TeleportSummary(results))

	if !dryRun {
		_ = nd.state.RecordTeleported()
		fmt.Println("  ✅ Teleport complete — your Windows files are now accessible from Linux")
	} else {
		fmt.Println("  (dry run — run without --dry-run to apply)")
	}
	fmt.Println()
	return nil
}

// ─── V15 Registry Commands ────────────────────────────────────────────

func runRegistryList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	_ = nd

	profiles, err := engine.ListRegistry(ctx)
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]string{"status": "error", "error": err.Error()})
		}
		return fmt.Errorf("registry: %w", err)
	}

	if outputJSON {
		return jsonOutput(map[string]interface{}{
			"status":   "ok",
			"profiles": profiles,
		})
	}

	if len(profiles) == 0 {
		fmt.Println("\n  ╔══════════════════════════════════════════════════╗")
		fmt.Println("  ║   NEXUS REGISTRY — NO PROFILES FOUND            ║")
		fmt.Println("  ╚══════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println("  The community registry is empty or unreachable.")
		fmt.Println("  Profiles will appear once contributors submit them.")
		fmt.Println()
		return nil
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║   NEXUS REGISTRY — COMMUNITY PROFILES           ║")
	fmt.Printf("  ║   %d profiles available                         ║\n", len(profiles))
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Print(engine.FormatRegistryProfiles(profiles))
	return nil
}

func runRegistrySearch(cmd *cobra.Command, args []string) error {
	query := args[0]
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	_ = nd

	results, err := engine.SearchRegistry(ctx, query)
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]string{"status": "error", "error": err.Error()})
		}
		return fmt.Errorf("registry: %w", err)
	}

	if outputJSON {
		return jsonOutput(map[string]interface{}{
			"status":  "ok",
			"query":   query,
			"results": results,
		})
	}

	if len(results) == 0 {
		fmt.Println()
		fmt.Printf("  No profiles match %q\n", query)
		fmt.Println("  Try a different search term or use 'nexus registry list'")
		fmt.Println()
		return nil
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Printf("  ║   SEARCH RESULTS: %q                ║\n", query)
	fmt.Printf("  ║   %d matching profiles                          ║\n", len(results))
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Print(engine.FormatRegistryProfiles(results))
	return nil
}

func runRegistryFetch(cmd *cobra.Command, args []string) error {
	name := args[0]
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	_ = nd

	data, err := engine.FetchRegistryProfile(ctx, name)
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]string{"status": "error", "error": err.Error()})
		}
		return fmt.Errorf("registry: %w", err)
	}

	// Save to profile store so 'nexus profile apply <name>' works immediately
	store, storeErr := initProfileStore()
	if storeErr == nil {
		_ = store.SaveProfile(name, data, manifest.SourceRemote)
	} else if !outputJSON {
		fmt.Fprintf(os.Stderr, "  ⚠️  Could not save to profile store: %v\n", storeErr)
	}

	if outputJSON {
		return jsonOutput(map[string]interface{}{
			"status": "ok",
			"name":   name,
			"size":   len(data),
			"sha256": fmt.Sprintf("%x", sha256.Sum256(data)),
		})
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Printf("  ║   FETCHED: %s\n", name)
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  Profile:  %s\n", name)
	fmt.Printf("  Size:     %d bytes\n", len(data))
	fmt.Printf("  SHA256:   %x\n", sha256.Sum256(data))
	fmt.Println()
	fmt.Println("  Profile saved to store. Run 'nexus profile apply <name>' to install.")
	fmt.Println()
	return nil
}

func runRegistrySubmit(cmd *cobra.Command, args []string) error {
	filePath := args[0]
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	_ = nd

	data, err := os.ReadFile(filePath)
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]string{"status": "error", "error": err.Error()})
		}
		return fmt.Errorf("registry: cannot read %s: %w", filePath, err)
	}

	instructions, err := engine.SubmitProfile(data)
	if err != nil {
		if outputJSON {
			return jsonOutput(map[string]string{"status": "error", "error": err.Error()})
		}
		return fmt.Errorf("registry: %w", err)
	}

	if outputJSON {
		return jsonOutput(map[string]interface{}{
			"status":       "ok",
			"file":         filePath,
			"size":         len(data),
			"instructions": instructions,
		})
	}

	fmt.Println()
	fmt.Println("  ╔══════════════════════════════════════════════════╗")
	fmt.Println("  ║   SUBMISSION READY                              ║")
	fmt.Println("  ╚══════════════════════════════════════════════════╝")
	fmt.Println()
	_ = instructions
	fmt.Println("  Profile validated and ready for community submission.")
	fmt.Println()
	fmt.Println("  To submit:")
	fmt.Println()
	fmt.Println("    1. Fork https://github.com/Sumama-Jameel/nexus-engine")
	fmt.Println("    2. Copy your profile to profiles/<name>.yaml")
	fmt.Println("    3. Add an entry to profiles/registry.json")
	fmt.Println("    4. Open a pull request")
	fmt.Println()
	fmt.Printf("  File: %s\n", filePath)
	fmt.Printf("  Size: %d bytes\n", len(data))
	fmt.Println()
	return nil
}
