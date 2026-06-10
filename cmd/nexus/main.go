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
        "encoding/json"
        "fmt"
        "os"
        "strings"

        "github.com/Sumama-Jameel/nexus-engine/cmd/nexus/runner"
        "github.com/Sumama-Jameel/nexus-engine/internal/bridge"
        "github.com/Sumama-Jameel/nexus-engine/internal/engine"
        "github.com/Sumama-Jameel/nexus-engine/internal/installer"
        "github.com/Sumama-Jameel/nexus-engine/internal/wsl"
        "github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
        "github.com/spf13/cobra"
        "github.com/spf13/viper"
)

var (
        outputJSON     bool
        initConfigPath string
        profilePath    string
        dryRun         bool
        forceRemove    bool
        wslDistroName  string
        wslSkipVerify  bool
        wslSkipDownload bool
        nexusVersion   = "0.6.0"
)

func main() {
        // Per Nexus Protocol Zero-Trust Architecture:
        // "Every system call the Go engine makes to the shell must pass
        // through a centralized SanitizeAndExecute function."
        // Route ALL bridge command execution through the security gate.
        bridge.SetExecFunc(bridge.ExecFunc(engine.SanitizeAndExecute))

        rootCmd := &cobra.Command{
                Use:   "nexus",
                Short: "Nexus Protocol — Unified Computing Layer",
                Long: `Nexus is a Unified Computing Layer designed to kill the friction 
of the desktop experience. It detects your OS, probes your hardware, 
and automates a perfect developer environment.

"If the Brain doesn't know where it is, it can't control anything."`,
                Version: nexusVersion,
        }

        rootCmd.PersistentFlags().BoolVar(&outputJSON, "json", false, "Output in JSON format (for IPC/API integration)")
        viper.AutomaticEnv()

        // ─── V1 Commands ───
        probeCmd := &cobra.Command{
                Use:   "probe",
                Short: "Probe the system — detect OS, hardware, and environment",
                RunE:  runProbe,
        }

        initCmd := &cobra.Command{
                Use:   "init",
                Short: "Initialize the Nexus environment — probe, validate, apply, configure, report",
                RunE:  runInit,
        }
        initCmd.Flags().StringVarP(&initConfigPath, "config", "c", "", "Path to custom nexus.yaml profile")
        initCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be installed without executing")

        versionCmd := &cobra.Command{
                Use:   "version",
                Short: "Print the Nexus Engine version",
                Run: func(cmd *cobra.Command, args []string) {
                        if outputJSON {
                                jsonOutput(map[string]string{"version": nexusVersion, "engine": "go"})
                        } else {
                                fmt.Printf("Nexus Engine v%s (Go)\n", nexusVersion)
                        }
                },
        }

        configCmd := &cobra.Command{Use: "config", Short: "Manage Nexus Engine configuration"}
        configGetCmd := &cobra.Command{
                Use:   "get [key]",
                Short: "Get a configuration value",
                Args:  cobra.ExactArgs(1),
                Run: func(cmd *cobra.Command, args []string) {
                        engine.InitConfig()
                        fmt.Println(viper.Get(args[0]))
                },
        }
        configCmd.AddCommand(configGetCmd)

        // ─── V2 Commands ───
        installCmd := &cobra.Command{
                Use:   "install [packages...]",
                Short: "Install packages via the Nexus Orchestrator",
                RunE:  runInstall,
        }
        installCmd.Flags().StringVarP(&profilePath, "profile", "p", "", "Install from a Nexus profile")
        installCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be installed without executing")

        removeCmd := &cobra.Command{
                Use:   "remove [packages...]",
                Short: "Remove Nexus-managed packages",
                Args:  cobra.MinimumNArgs(1),
                RunE:  runRemove,
        }

        listCmd := &cobra.Command{
                Use:   "list",
                Short: "List Nexus-managed packages",
                RunE:  runList,
        }

        searchCmd := &cobra.Command{
                Use:   "search [query]",
                Short: "Search for available packages",
                Args:  cobra.ExactArgs(1),
                RunE:  runSearch,
        }

        updateCmd := &cobra.Command{
                Use:   "update [packages...]",
                Short: "Update Nexus-managed packages",
                RunE:  runUpdate,
        }

        // ─── V3 Commands: Profile Management ───
        profileCmd := &cobra.Command{
                Use:   "profile",
                Short: "Manage Nexus profiles — create, validate, fetch, and inspect YAML manifests",
                Long: `Nexus profiles are declarative YAML manifests that define your computing environment.
Per the Nexus Protocol: "We shift from Hardcoded to Declarative."
The user just edits a text file to change their OS.`,
        }

        profileListCmd := &cobra.Command{
                Use:   "list",
                Short: "List all available Nexus profiles",
                Long:  `Lists profiles from the local store with source, version, and integrity hash.`,
                RunE:  runProfileList,
        }

        profileShowCmd := &cobra.Command{
                Use:   "show <name>",
                Short: "Show profile content and metadata",
                Args:  cobra.ExactArgs(1),
                RunE:  runProfileShow,
        }

        profileValidateCmd := &cobra.Command{
                Use:   "validate <file>",
                Short: "Validate a profile YAML file against the Nexus Schema",
                Long: `Validates a YAML file against the embedded JSON Schema and Go semantic rules.
Exit code 0 = valid, 1 = invalid. Use in CI/CD pipelines (GitHub Actions).`,
                Args: cobra.ExactArgs(1),
                RunE: runProfileValidate,
        }

        profileCreateCmd := &cobra.Command{
                Use:   "create <name>",
                Short: "Create a new Nexus profile interactively",
                Long:  `Interactive wizard that generates a valid YAML profile with schema validation.`,
                Args:  cobra.ExactArgs(1),
                RunE:  runProfileCreate,
        }

        profileFetchCmd := &cobra.Command{
                Use:   "fetch <name>",
                Short: "Fetch a profile from the remote community repository",
                Long: `Downloads a profile from GitHub (the Community Ledger).
The profile is validated against the schema before saving.
Network errors are non-fatal — local profiles always work.`,
                Args: cobra.ExactArgs(1),
                RunE: runProfileFetch,
        }

        profileRemoveCmd := &cobra.Command{
                Use:   "remove <name>",
                Short: "Remove a profile from the local store",
                Args:  cobra.ExactArgs(1),
                RunE:  runProfileRemove,
        }
        profileRemoveCmd.Flags().BoolVar(&forceRemove, "force", false, "Allow removing bundled profiles")

        profileVerifyCmd := &cobra.Command{
                Use:   "verify <name>",
                Short: "Verify a profile's SHA256 integrity",
                Long:  `Recomputes the SHA256 hash and compares it to the registry. Detects tampering.`,
                Args:  cobra.ExactArgs(1),
                RunE:  runProfileVerify,
        }

        profileApplyCmd := &cobra.Command{
                Use:   "apply <name>",
                Short: "Apply a profile — install its packages via the Orchestrator",
                Long: `Applies a named profile by resolving its target for the current
package manager, then executing the full Orchestrator flow:
PreFlight → RefreshIndex → Order → Execute → Verify → Record → Audit → Report.
This is the declarative equivalent of 'nexus install --profile <name>'.`,
                Args: cobra.ExactArgs(1),
                RunE: runProfileApply,
        }
        profileApplyCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be installed without executing")

        profileCmd.AddCommand(profileListCmd, profileShowCmd, profileValidateCmd,
                profileCreateCmd, profileFetchCmd, profileRemoveCmd, profileVerifyCmd,
                profileApplyCmd)

        // ─── V4 Commands: WSL2 Detection ───
        wslCmd := &cobra.Command{
                Use:   "wsl",
                Short: "WSL2 detection, management, and import — The Spy & The Bridge",
                Long: `Per V4 "The WSL2 Detector (The Spy)" and V5 "The Instant Linux Importer (The Bridge)":
Detect WSL2 readiness, import Linux rootfs images, and manage WSL2 instances.
On Linux, WSL2 import commands are not available — use 'nexus probe' instead.`,
        }

        wslStatusCmd := &cobra.Command{
                Use:   "status",
                Short: "Display full WSL2 detection report",
                Long:  `Comprehensive WSL2 status: Windows version, WSL availability, installed distributions, Hyper-V status, readiness check.`,
                RunE:  runWSLStatus,
        }

        wslCheckCmd := &cobra.Command{
                Use:   "check",
                Short: "Check if the system is ready for Nexus WSL2 setup",
                Long: `Quick readiness check — exits 0 if ready, 1 if not.
Useful for scripting and CI/CD pipelines.`,
                RunE:  runWSLCheck,
        }

        // ─── V5 Commands: WSL2 Import (The Bridge) ───
        wslImportCmd := &cobra.Command{
                Use:   "import [image]",
                Short: "Download and import a Linux rootfs into WSL2",
                Long: `Per V5 "The Instant Linux Importer (The Bridge)":
Downloads a tiny Linux image and imports it into WSL2 automatically.
Within 60 seconds, you have a Linux engine running inside Windows.

Available images: nexus-alpine (minimal, ~3MB), nexus-debian (full dev, ~120MB).
If no image is specified, defaults to nexus-alpine (the 60-second promise).`,
                RunE: runWSLImport,
        }
        wslImportCmd.Flags().StringVar(&wslDistroName, "name", "Nexus", "Custom name for the WSL2 distribution")
        wslImportCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would happen without executing")
        wslImportCmd.Flags().BoolVar(&wslSkipVerify, "skip-verify", false, "Skip SHA256 verification (DANGEROUS)")
        wslImportCmd.Flags().BoolVar(&wslSkipDownload, "skip-download", false, "Use local tarball (for offline/air-gapped installs)")

        wslSetupCmd := &cobra.Command{
                Use:   "setup",
                Short: "One-command full WSL2 setup — the 60-second promise",
                Long: `Per V5: "The user clicks one button, and suddenly they have a Linux engine running inside Windows."

This command performs the complete setup:
  1. Checks WSL2 readiness (reuses V4's Spy detection)
  2. Downloads the default Nexus Alpine rootfs (~3MB)
  3. Imports it into WSL2 with security-hardened wsl.conf
  4. Creates a non-root user and configures the environment
  5. Records the instance for Nexus state management

This is the simplest entry point for new users.`,
                RunE: runWSLSetup,
        }
        wslSetupCmd.Flags().StringVar(&wslDistroName, "name", "Nexus", "Custom name for the WSL2 distribution")
        wslSetupCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would happen without executing")

        wslRemoveCmd := &cobra.Command{
                Use:   "remove [distro]",
                Short: "Remove a Nexus-managed WSL2 distribution",
                Long:  `Removes a WSL2 distribution that was imported by Nexus. Only Nexus-managed distros can be removed.`,
                Args:  cobra.ExactArgs(1),
                RunE:  runWSLRemove,
        }
        wslRemoveCmd.Flags().BoolVar(&forceRemove, "force", false, "Skip confirmation prompt")

        wslListCmd := &cobra.Command{
                Use:   "list",
                Short: "List Nexus-managed WSL2 distributions",
                Long:  `Lists WSL2 distributions managed by Nexus (distinct from 'wsl --list' which shows all distros).`,
                RunE:  runWSLList,
        }

        wslEnterCmd := &cobra.Command{
                Use:   "enter [distro]",
                Short: "Enter a Nexus-managed WSL2 distribution",
                Long: `Launches an interactive shell inside a Nexus-managed WSL2 distribution.
This is the simplest way to start using your imported Linux environment.
Defaults to the distribution named "Nexus" if no name is specified.`,
                RunE: runWSLEnter,
        }
        wslEnterCmd.Flags().StringVar(&wslDistroName, "name", "Nexus", "Name of the WSL2 distribution to enter")

        wslImagesCmd := &cobra.Command{
                Use:   "images",
                Short: "List available rootfs images for import",
                Long:  `Shows the built-in rootfs image registry with names, sizes, and descriptions.`,
                RunE:  runWSLImages,
        }

        wslCmd.AddCommand(wslStatusCmd, wslCheckCmd,
                wslImportCmd, wslSetupCmd, wslRemoveCmd, wslListCmd, wslEnterCmd, wslImagesCmd)

        rootCmd.AddCommand(probeCmd, initCmd, versionCmd, configCmd,
                installCmd, removeCmd, listCmd, searchCmd, updateCmd, profileCmd,
                wslCmd)

        if err := rootCmd.Execute(); err != nil {
                os.Exit(1)
        }
}

// ─── Helpers ───

type nexusDeps struct {
        pm     installer.PackageManager
        state  *engine.StateTracker
        audit  *engine.AuditLogger
        env    *bridge.EnvironmentInfo
        family string
        execFn installer.ExecFunc
}

func initDeps(ctx context.Context) (*nexusDeps, error) {
        env := bridge.DetectEnvironment(ctx)

        pmToFamily := map[string]string{
                "apt": "debian", "pacman": "arch", "dnf": "fedora", "yum": "fedora", "apk": "alpine",
        }
        family := pmToFamily[env.PackageManager]
        if family == "" {
                return nil, fmt.Errorf("unsupported package manager: %s", env.PackageManager)
        }

        pm, err := installer.NewInstaller(family, engine.SanitizeAndExecute)
        if err != nil {
                return nil, fmt.Errorf("failed to create installer: %w", err)
        }

        state, err := engine.NewStateTracker()
        if err != nil {
                return nil, fmt.Errorf("failed to init state tracker: %w", err)
        }

        audit, err := engine.NewAuditLogger()
        if err != nil {
                return nil, fmt.Errorf("failed to init audit logger: %w", err)
        }

        return &nexusDeps{
                pm: pm, state: state, audit: audit, env: env,
                family: family, execFn: engine.SanitizeAndExecute,
        }, nil
}

func initProfileStore() (*manifest.ProfileStore, error) {
        store, err := manifest.NewProfileStore()
        if err != nil {
                return nil, err
        }
        // Initialize bundled defaults
        if err := store.Initialize(manifest.BundledDefaults()); err != nil {
                return nil, fmt.Errorf("failed to initialize profile store: %w", err)
        }
        return store, nil
}

// initRunnerDeps creates a runner.Dependencies from the real system dependencies.
// Per the Humble Object pattern: the runner holds the business logic,
// main.go is just the thin CLI wiring/formatting layer.
func initRunnerDeps(ctx context.Context) (*runner.Dependencies, error) {
        deps, err := initDeps(ctx)
        if err != nil {
                return nil, err
        }

        store, storeErr := initProfileStore()
        if storeErr != nil {
                // Non-fatal: runner can work without profile store for basic operations
                store = nil
        }

        return &runner.Dependencies{
                PM:           deps.pm,
                State:        deps.state,
                Audit:        deps.audit,
                Env:          deps.env,
                Family:       deps.family,
                ExecFn:       deps.execFn,
                ProfileStore: store,
                Output:       os.Stdout,
                JSONOutput:   outputJSON,
                DryRun:       dryRun,
                ForceRemove:  forceRemove,
        }, nil
}

// ─── V1 Commands ───

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
                fmt.Println("  ╚══════════════════════════════════════════════════╝")
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

        // STEP 5: REPORT
        if !outputJSON {
                fmt.Println("  [5/5] 📊 Generating report...")
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

                fmt.Println("  ╔══════════════════════════════════════════════════╗")
                fmt.Println("  ║   ✅ NEXUS INIT COMPLETE — SYSTEM READY          ║")
                fmt.Println("  ╚══════════════════════════════════════════════════╝")
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

        return nil
}

// ─── V2 Commands ───

func runInstall(cmd *cobra.Command, args []string) error {
        ctx := context.Background()

        rdeps, err := initRunnerDeps(ctx)
        if err != nil {
                return err
        }
        defer rdeps.Audit.Close()

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
        defer rdeps.Audit.Close()

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
        defer rdeps.Audit.Close()

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
        defer rdeps.Audit.Close()

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
                        fmt.Printf("  ⛔ Could not read profile: %v\n", err)
                } else {
                        fmt.Println("  ── RAW CONTENT ──────────────────────────────")
                        fmt.Println(content)
                }
        }

        fmt.Println()
        return nil
}

func runProfileValidate(cmd *cobra.Command, args []string) error {
        filePath := args[0]

        ctx := context.Background()
        rdeps, err := initRunnerDeps(ctx)
        if err != nil {
                return err
        }

        data, readErr := os.ReadFile(filePath)
        if readErr != nil {
                fmt.Fprintf(os.Stderr, "  ⛔ Failed to read file: %v\n", readErr)
                os.Exit(1)
        }

        profile, validateErr := rdeps.ValidateProfileBytes(data)
        if validateErr != nil {
                if outputJSON {
                        jsonOutput(map[string]interface{}{"valid": false, "error": validateErr.Error()})
                } else {
                        fmt.Fprintf(os.Stderr, "  ⛔ INVALID: %v\n", validateErr)
                }
                os.Exit(1)
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
                return nil
        }

        // Serialize
        yamlContent, err := manifest.FormatProfileYAML(profile)
        if err != nil {
                return fmt.Errorf("failed to serialize profile: %w", err)
        }

        // Schema validate
        if _, err := manifest.ParseBytes([]byte(yamlContent)); err != nil {
                fmt.Fprintf(os.Stderr, "  ⛔ Schema validation failed: %v\n", err)
                return nil
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
                        jsonOutput(map[string]interface{}{"success": false, "error": err.Error()})
                } else {
                        fmt.Fprintf(os.Stderr, "  ⛔ Fetch failed: %v\n", err)
                }
                return nil
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
                        jsonOutput(map[string]interface{}{"success": false, "error": err.Error()})
                } else {
                        fmt.Fprintf(os.Stderr, "  ⛔ %v\n", err)
                }
                return nil
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
                        jsonOutput(map[string]interface{}{"valid": false, "error": err.Error()})
                } else {
                        fmt.Fprintf(os.Stderr, "  ⛔ INTEGRITY CHECK FAILED: %v\n", err)
                }
                os.Exit(1)
        }

        meta, _ := store.GetMeta(name)
        if outputJSON {
                return jsonOutput(map[string]interface{}{"valid": true, "name": name, "sha256": meta.SHA256})
        }

        fmt.Printf("  ✅ Profile '%s' integrity verified (SHA256: %s…)\n", name, meta.SHA256[:32])
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
        defer rdeps.Audit.Close()
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
                _ = jsonOutput(map[string]interface{}{
                        "ready":     status.Ready,
                        "blockers":  status.Blockers,
                })
                if !status.Ready {
                        os.Exit(1)
                }
                return nil
        }

        fmt.Print(bridge.FormatWSL2Check(status))

        if !status.Ready {
                os.Exit(1)
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
                        jsonOutput(map[string]interface{}{"error": errMsg, "available": false})
                } else {
                        fmt.Printf("\n  ⛔ %s\n\n", errMsg)
                }
                os.Exit(1)
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
                        jsonOutput(map[string]interface{}{"error": err.Error(), "available_images": wsl.DefaultRootFSRegistry()})
                } else {
                        fmt.Printf("\n  ⛔ %v\n\n", err)
                }
                os.Exit(1)
        }

        // Validate distro name
        if err := wsl.ValidateDistroName(wslDistroName); err != nil {
                if outputJSON {
                        jsonOutput(map[string]interface{}{"error": fmt.Sprintf("invalid distro name: %v", err)})
                } else {
                        fmt.Printf("\n  ⛔ Invalid distro name: %v\n\n", err)
                }
                os.Exit(1)
        }

        // Determine install path
        homeDir, _ := os.UserHomeDir()
        installPath := homeDir + "/.nexus/wsl/" + wslDistroName

        // Validate install path
        if err := wsl.ValidateInstallPath(installPath); err != nil {
                if outputJSON {
                        jsonOutput(map[string]interface{}{"error": fmt.Sprintf("invalid install path: %v", err)})
                } else {
                        fmt.Printf("\n  ⛔ Invalid install path: %v\n\n", err)
                }
                os.Exit(1)
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
                        audit.Log(entry)
                })
                defer audit.Close()
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
                        jsonOutput(map[string]interface{}{"error": errMsg, "available": false})
                } else {
                        fmt.Printf("\n  ⛔ %s\n\n", errMsg)
                }
                os.Exit(1)
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
                        jsonOutput(map[string]interface{}{
                                "ready": false, "blockers": status.Blockers,
                                "recommendations": status.Recommendations,
                        })
                } else {
                        fmt.Println("  ⛔ System is NOT ready for WSL2 setup")
                        fmt.Print(bridge.FormatWSL2Check(status))
                }
                os.Exit(1)
        }

        if !outputJSON {
                fmt.Println("  ✅ WSL2 readiness confirmed")
        }

        // Step 2: Import with default Alpine image
        image, err := wsl.FindImage("nexus-alpine")
        if err != nil {
                if outputJSON {
                        jsonOutput(map[string]interface{}{"error": err.Error()})
                } else {
                        fmt.Printf("  ⛔ %v\n", err)
                }
                os.Exit(1)
        }

        if err := wsl.ValidateDistroName(wslDistroName); err != nil {
                if outputJSON {
                        jsonOutput(map[string]interface{}{"error": fmt.Sprintf("invalid distro name: %v", err)})
                } else {
                        fmt.Printf("  ⛔ Invalid distro name: %v\n", err)
                }
                os.Exit(1)
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
                        audit.Log(entry)
                })
                defer audit.Close()
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
                        jsonOutput(map[string]interface{}{"error": "WSL2 management is only available on Windows"})
                } else {
                        fmt.Println("\n  ⛔ WSL2 management is only available on Windows")
                }
                os.Exit(1)
        }

        state, _ := engine.NewStateTracker()
        isManaged := false
        if state != nil {
                isManaged = state.IsWSLManaged(distroName)
        }

        if !isManaged && !forceRemove {
                if outputJSON {
                        jsonOutput(map[string]interface{}{
                                "error":   fmt.Sprintf("'%s' is not a Nexus-managed WSL2 distribution. Use --force to remove anyway.", distroName),
                                "managed": false,
                        })
                } else {
                        fmt.Printf("\n  ⚠️  '%s' is not a Nexus-managed WSL2 distribution.\n", distroName)
                        fmt.Println("  Use --force to remove anyway.")
                        fmt.Println()
                }
                os.Exit(1)
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
                        audit.Log(entry)
                })
                defer audit.Close()
        }

        if err := wslImporter.Remove(ctx, distroName, forceRemove); err != nil {
                if outputJSON {
                        jsonOutput(map[string]interface{}{"error": err.Error()})
                } else {
                        fmt.Printf("\n  ⛔ Failed to remove '%s': %v\n\n", distroName, err)
                }
                os.Exit(1)
        }

        if state != nil {
                state.RecordWSLRemove(distroName)
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
                        jsonOutput(map[string]interface{}{"error": errMsg, "available": false})
                } else {
                        fmt.Printf("\n  ⛔ %s\n\n", errMsg)
                }
                os.Exit(1)
        }

        // Allow distro name from positional arg or --name flag
        distroName := wslDistroName
        if len(args) > 0 {
                distroName = args[0]
        }

        if err := wsl.ValidateDistroName(distroName); err != nil {
                if outputJSON {
                        jsonOutput(map[string]interface{}{"error": fmt.Sprintf("invalid distro name: %v", err)})
                } else {
                        fmt.Printf("\n  ⛔ Invalid distro name: %v\n\n", err)
                }
                os.Exit(1)
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
                        jsonOutput(map[string]interface{}{"error": err.Error()})
                } else {
                        fmt.Printf("\n  ⛔ Failed to enter distribution '%s': %v\n\n", distroName, err)
                }
                os.Exit(1)
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
