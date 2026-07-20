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

	"github.com/Sumama-Jameel/nexus-engine/internal/bridge"
	"github.com/Sumama-Jameel/nexus-engine/internal/dotfiles"
	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
	"github.com/Sumama-Jameel/nexus-engine/internal/ledger"
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
