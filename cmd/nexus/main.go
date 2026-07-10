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
	"os/signal"
	"syscall"

	"github.com/Sumama-Jameel/nexus-engine/cmd/nexus/runner"
	"github.com/Sumama-Jameel/nexus-engine/internal/bridge"
	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	outputJSON             bool
	initConfigPath         string
	profilePath            string
	dryRun                 bool
	forceRemove            bool
	wslDistroName          string
	wslSkipVerify          bool
	wslSkipDownload        bool
	vaultForce             bool
	yesMode                bool
	allowUnlistedServices  bool
	nexusVersion          = "0.16.0"
)

func main() {
	// Create a context that cancels on SIGINT/SIGTERM for graceful shutdown.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

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
		RunE: func(cmd *cobra.Command, args []string) error {
			if outputJSON {
				return jsonOutput(map[string]string{"version": nexusVersion, "engine": "go"})
			}
			fmt.Printf("Nexus Engine v%s (Go)\n", nexusVersion)
			return nil
		},
	}

	configCmd := &cobra.Command{Use: "config", Short: "Manage Nexus Engine configuration"}
	configGetCmd := &cobra.Command{
		Use:   "get [key]",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			allowedKeys := map[string]bool{
				"profile": true, "package_manager": true, "shell": true,
				"auto_update": true, "verbose": true,
			}
			if !allowedKeys[args[0]] {
				return fmt.Errorf("unknown config key: %s", args[0])
			}
			if _, err := engine.InitConfig(); err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			fmt.Println(viper.Get(args[0]))
			return nil
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

	profileSuggestCmd := &cobra.Command{
		Use:   "suggest",
		Short: "Suggest profiles based on your hardware and distro",
		Long: `V16: Analyzes your system's hardware (GPU, CPU, RAM) and distro,
then recommends the best-matching profiles. Shows compatibility warnings
when a profile is designed for a different distro.`,
		RunE: runProfileSuggest,
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
		profileSuggestCmd, profileApplyCmd)

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
		RunE: runWSLCheck,
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

		// ─── V7 Commands: Dotfiles Management ───
		dotfilesCmd := &cobra.Command{
			Use:   "dotfiles",
			Short: "Manage dotfiles via Chezmoi — install, sync, vault",
			Long:  `Per V7 "The Chezmoi Integration (The Memory)".`,
		}

		dotfilesDetectCmd := &cobra.Command{
			Use:   "detect",
			Short: "Detect Chezmoi installation",
			RunE:  runDotfilesDetect,
		}

		dotfilesInstallCmd := &cobra.Command{
			Use:   "install",
			Short: "Install Chezmoi",
			RunE:  runDotfilesInstall,
		}

		dotfilesInitCmd := &cobra.Command{
			Use:   "init <repo>",
			Short: "Initialize chezmoi with a Git repo",
			Args:  cobra.ExactArgs(1),
			RunE:  runDotfilesInit,
		}

		dotfilesRemoveCmd := &cobra.Command{
			Use:   "remove",
			Short: "Remove the Chezmoi installation",
			RunE:  runDotfilesRemove,
		}

		dotfilesApplyCmd := &cobra.Command{
			Use:   "apply",
			Short: "Apply dotfiles to live system",
			RunE:  runDotfilesApply,
		}

		dotfilesStatusCmd := &cobra.Command{
			Use:   "status",
			Short: "Show chezmoi status",
			RunE:  runDotfilesStatus,
		}

		dotfilesDiffCmd := &cobra.Command{
			Use:   "diff",
			Short: "Show pending dotfile changes",
			RunE:  runDotfilesDiff,
		}

		dotfilesAddCmd := &cobra.Command{
			Use:   "add <path>",
			Short: "Add a file to managed dotfiles",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				force, _ := cmd.Flags().GetBool("force")
				return runDotfilesAdd(cmd, args, force)
			},
		}
		dotfilesAddCmd.Flags().Bool("force", false, "Allow tracking sensitive paths (--force)")

		dotfilesVerifyCmd := &cobra.Command{
			Use:   "verify",
			Short: "Verify managed dotfiles match source",
			RunE:  runDotfilesVerify,
		}

		dotfilesPushCmd := &cobra.Command{
			Use:   "push",
			Short: "Push local changes to remote",
			RunE: func(cmd *cobra.Command, args []string) error {
				message, _ := cmd.Flags().GetString("message")
				force, _ := cmd.Flags().GetBool("force")
				token, _ := cmd.Flags().GetString("token")
				return runDotfilesPush(cmd, message, force, token)
			},
		}
		dotfilesPushCmd.Flags().String("message", "Update from Nexus", "Commit message")
		dotfilesPushCmd.Flags().Bool("force", false, "Skip secret scan")
		dotfilesPushCmd.Flags().String("token", "", "GitHub PAT (overrides NEXUS_DOTFILES_TOKEN env)")

		dotfilesPullCmd := &cobra.Command{
			Use:   "pull",
			Short: "Pull from remote",
			RunE: func(cmd *cobra.Command, args []string) error {
				rebase, _ := cmd.Flags().GetBool("rebase")
				token, _ := cmd.Flags().GetString("token")
				return runDotfilesPull(cmd, rebase, token)
			},
		}
		dotfilesPullCmd.Flags().Bool("rebase", false, "Use rebase instead of --ff-only")
		dotfilesPullCmd.Flags().String("token", "", "GitHub PAT (overrides NEXUS_DOTFILES_TOKEN env)")

		dotfilesSyncCmd := &cobra.Command{
			Use:   "sync",
			Short: "Pull + apply + push",
			RunE: func(cmd *cobra.Command, args []string) error {
				message, _ := cmd.Flags().GetString("message")
				force, _ := cmd.Flags().GetBool("force")
				token, _ := cmd.Flags().GetString("token")
				return runDotfilesSync(cmd, message, force, token)
			},
		}
		dotfilesSyncCmd.Flags().String("message", "Update from Nexus", "Commit message")
		dotfilesSyncCmd.Flags().Bool("force", false, "Skip secret scan")
		dotfilesSyncCmd.Flags().String("token", "", "GitHub PAT (overrides NEXUS_DOTFILES_TOKEN env)")

		dotfilesCmd.AddCommand(dotfilesDetectCmd, dotfilesInstallCmd, dotfilesInitCmd, dotfilesRemoveCmd,
			dotfilesApplyCmd, dotfilesStatusCmd, dotfilesDiffCmd,
			dotfilesAddCmd, dotfilesVerifyCmd,
			dotfilesPushCmd, dotfilesPullCmd, dotfilesSyncCmd)

		// ─── V9 Commands: Secrets Vault ───
		vaultCmd := &cobra.Command{
			Use:   "vault",
			Short: "Age-encrypted secrets vault — add, list, remove",
			Long:  `Per V9 "The Secrets Vault (The Shield)".`,
		}

		vaultInitCmd := &cobra.Command{
			Use:   "init",
			Short: "Initialize age key pair",
			RunE: func(cmd *cobra.Command, args []string) error {
				force, _ := cmd.Flags().GetBool("force")
				return runDotfilesVaultInit(cmd, force)
			},
		}
		vaultInitCmd.Flags().Bool("force", false, "Re-initialize even if vault already exists")

		vaultAddCmd := &cobra.Command{
			Use:   "add <file>",
			Short: "Encrypt a file with age",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				force, _ := cmd.Flags().GetBool("force")
				return runDotfilesVaultAdd(cmd, args[0], force)
			},
		}
		vaultAddCmd.Flags().Bool("force", false, "Force re-encryption of an already-encrypted file")

		vaultListCmd := &cobra.Command{
			Use:   "list",
			Short: "List vault-encrypted files",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runDotfilesVaultList(cmd)
			},
		}

		vaultStatusCmd := &cobra.Command{
			Use:   "status",
			Short: "Show vault status",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runDotfilesVaultStatus(cmd)
			},
		}

		vaultUnlockCmd := &cobra.Command{
			Use:   "unlock",
			Short: "Unlock vault with a private key",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runDotfilesVaultUnlock(cmd, args)
			},
		}

		vaultRemoveCmd := &cobra.Command{
			Use:   "remove <file>",
			Short: "Remove a vault entry",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return runDotfilesVaultRemove(cmd, args[0], vaultForce)
			},
		}
		vaultRemoveCmd.Flags().BoolVar(&vaultForce, "force", false, "Skip confirmation")

		vaultCmd.AddCommand(vaultInitCmd, vaultAddCmd, vaultListCmd, vaultStatusCmd,
			vaultUnlockCmd, vaultRemoveCmd)

		dotfilesCmd.AddCommand(dotfilesDetectCmd, dotfilesInstallCmd, dotfilesInitCmd, dotfilesRemoveCmd,
			dotfilesApplyCmd, dotfilesStatusCmd, dotfilesDiffCmd,
			dotfilesAddCmd, dotfilesVerifyCmd,
			dotfilesPushCmd, dotfilesPullCmd, dotfilesSyncCmd,
			vaultCmd)

		// ─── V11 Commands: Mode Switcher ───
		modeCmd := &cobra.Command{
			Use:   "mode",
			Short: "Atomic mode switching — dev, gamer, work, or custom",
			Long: `Modes are declarative, switchable units that bundle a profile +
	dotfiles + service toggles + OS tweaks into one atomic apply call.
	See ADR 010 for the full architecture.`,
		}

		modeListCmd := &cobra.Command{
			Use:   "list",
			Short: "List all available modes (built-ins + user-defined)",
			RunE:  runModeList,
		}

		modeCurrentCmd := &cobra.Command{
			Use:   "current",
			Short: "Show the currently active mode",
			RunE:  runModeCurrent,
		}

		modeShowCmd := &cobra.Command{
			Use:   "show <name>",
			Short: "Show a mode's definition (profile, services, tweaks)",
			Args:  cobra.ExactArgs(1),
			RunE:  runModeShow,
		}

		modeApplyCmd := &cobra.Command{
			Use:   "apply <name>",
			Short: "Apply a mode — switch profile, services, and OS tweaks atomically",
			Long: `Applies a named mode through the V11 atomic switch pipeline:
	  1. Resolve the mode (built-in or user-defined)
	  2. Validate services against the allowlist
	  3. Apply the referenced profile
	  4. (if dotfiles_source set) re-bind chezmoi
	  5. Stop listed services, start listed services
	  6. Apply OS tweaks (cpu governor, power plan)
	  7. Audit the switch

	  Dry-run prints the plan without executing.
	  --allow-unlisted-services lifts the service allowlist (audit-logged).`,
			Args: cobra.ExactArgs(1),
			RunE: runModeApply,
		}
		modeApplyCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show the switch plan without executing")
		modeApplyCmd.Flags().BoolVar(&yesMode, "yes", false, "Skip confirmation prompt")
		modeApplyCmd.Flags().BoolVar(&allowUnlistedServices, "allow-unlisted-services", false, "Allow service names not in the allowlist (audit-logged)")

		modeRollbackCmd := &cobra.Command{
			Use:   "rollback",
			Short: "Re-apply the previously active mode",
			Long: `Re-applies the mode that was active before the current one.
	Equivalent to 'nexus mode apply <previous>' — useful when an apply
	fails partway and the system is in an inconsistent state.
	No-op when no previous mode is recorded.`,
			RunE: runModeRollback,
		}

		modeDefineCmd := &cobra.Command{
			Use:   "define <name>",
			Short: "Define a new user mode interactively",
			Long: `Interactive wizard that walks through all mode fields and
	writes the YAML to ~/.nexus/modes/<name>.yaml.`,
			Args: cobra.ExactArgs(1),
			RunE: runModeDefine,
		}

		modeCmd.AddCommand(modeListCmd, modeCurrentCmd, modeShowCmd,
			modeApplyCmd, modeRollbackCmd, modeDefineCmd)

		// ─── V12 Commands: Container Management ───
		containerCmd := &cobra.Command{
			Use:   "container",
			Short: "Distrobox — run any Linux app from any distro",
			Long: `Manage Distrobox containers. Create, enter, list, and remove
	containers running any Linux distribution.
	See ADR 011 for the full architecture.`,
		}

		containerListCmd := &cobra.Command{
			Use:   "list",
			Short: "List all Distrobox containers",
			RunE:  runContainerList,
		}

		containerInfoCmd := &cobra.Command{
			Use:   "info <name>",
			Short: "Show container details",
			Args:  cobra.ExactArgs(1),
			RunE:  runContainerInfo,
		}

		containerCreateCmd := &cobra.Command{
			Use:   "create <name>",
			Short: "Create a Distrobox container",
			Args:  cobra.ExactArgs(1),
			RunE:  runContainerCreate,
		}
		containerCreateCmd.Flags().String("image", "", "OCI image reference")

		containerEnterCmd := &cobra.Command{
			Use:   "enter <name>",
			Short: "Print the command to enter a container",
			Args:  cobra.ExactArgs(1),
			RunE:  runContainerEnter,
		}

		containerAppsCmd := &cobra.Command{
			Use:   "apps <name>",
			Short: "List apps in a container",
			Args:  cobra.ExactArgs(1),
			RunE:  runContainerApps,
		}

		containerRemoveCmd := &cobra.Command{
			Use:   "remove <name>",
			Short: "Remove a container",
			Args:  cobra.ExactArgs(1),
			RunE:  runContainerRemove,
		}
		containerRemoveCmd.Flags().Bool("force", false, "Skip managed-state check")

		containerCmd.AddCommand(containerListCmd, containerInfoCmd,
			containerCreateCmd, containerEnterCmd, containerAppsCmd, containerRemoveCmd)

		// ─── V13 Commands: Hardware Ledger ───
		ledgerCmd := &cobra.Command{
			Use:   "ledger",
			Short: "Hardware compatibility ledger — record, query, and sync hardware reports",
			Long: `The V13 Hardware Ledger (The Intelligence) records hardware configuration
	snapshots paired with install outcomes. Use it to understand what works
	on your machine and to contribute to the community compatibility database.`,
		}

		ledgerRecordCmd := &cobra.Command{
			Use:   "record",
			Short: "Record a hardware report from the current system state",
			Long:  `Probes the system and saves a hardware report to the local ledger.`,
			RunE:  runLedgerRecord,
		}

		ledgerStatusCmd := &cobra.Command{
			Use:   "status",
			Short: "Show ledger statistics and sync status",
			Long:  `Displays the number of records, last record time, and sync status.`,
			RunE:  runLedgerStatus,
		}

		ledgerQueryCmd := &cobra.Command{
			Use:   "query <field> <value>",
			Short: "Query the ledger for hardware compatibility data",
			Long: `Search the ledger for records matching a specific field value.
	Supported fields: gpu, kernel, os, arch, cpu.
	Example: nexus ledger query gpu "NVIDIA"`,
			Args: cobra.ExactArgs(2),
			RunE: runLedgerQuery,
		}

		ledgerCheckCmd := &cobra.Command{
			Use:   "check",
			Short: "Check if your hardware is known to work with Nexus",
			Long:  `Probes the system and compares against the ledger to see if this hardware has been tested.`,
			RunE:  runLedgerCheck,
		}

		ledgerSyncCmd := &cobra.Command{
			Use:   "sync",
			Short: "Push local ledger to the community registry",
			Long: `Uploads anonymized hardware reports to the community ledger.
	Use --enable to opt into community sharing on first sync.
	Data contains NO personal information — only hardware specs + success status.`,
			RunE: runLedgerSync,
		}
		ledgerSyncCmd.Flags().Bool("enable", false, "Enable community sync and upload local records")
		ledgerSyncCmd.Flags().Bool("disable", false, "Disable community sync")

		ledgerPullCmd := &cobra.Command{
			Use:   "pull",
			Short: "Pull community compatibility data",
			Long:  `Downloads the community compatibility registry for offline matching.`,
			RunE:  runLedgerPull,
		}

		ledgerCmd.AddCommand(ledgerRecordCmd, ledgerStatusCmd, ledgerQueryCmd,
			ledgerCheckCmd, ledgerSyncCmd, ledgerPullCmd)

		// ─── V14 Commands: Teleport Migration ───
		teleportCmd := &cobra.Command{
			Use:   "teleport",
			Short: "Migrate Windows user folders into WSL2 via symlinks",
			Long: `The V14 Teleport Migration Tool (The Closer) walks your Windows
	user profile (Documents, Desktop, Downloads, Pictures) and creates
	symlinks in your WSL2 home directory — zero data copy, zero risk.

	Only available inside WSL2. On native Linux this command is a no-op.`,
			RunE: runTeleport,
		}
		teleportCmd.Flags().Bool("dry-run", false, "Preview what would be linked without making changes")

		// ─── V15 Commands: Global Registry ───
		registryCmd := &cobra.Command{
			Use:   "registry",
			Short: "Browse, fetch, and submit community profiles",
			Long: `The V15 Global Registry (The Launch) connects you to the
	community profile ecosystem. Browse available profiles, fetch them
	into your local store, and submit your own for others to use.

	The registry is hosted in the profiles/ directory of this repository.`,
		}

		registryListCmd := &cobra.Command{
			Use:   "list",
			Short: "List all available profiles in the community registry",
			Long:  `Fetches the community registry index and displays all profiles.`,
			RunE:  runRegistryList,
		}

		registrySearchCmd := &cobra.Command{
			Use:   "search <query>",
			Short: "Search the registry for profiles by keyword",
			Long:  `Searches profile names, authors, descriptions, and target families.`,
			Args:  cobra.ExactArgs(1),
			RunE:  runRegistrySearch,
		}

		registryFetchCmd := &cobra.Command{
			Use:   "fetch <name>",
			Short: "Download a profile from the registry",
			Long:  `Fetches a profile by name, verifies its SHA256, and saves it locally.`,
			Args:  cobra.ExactArgs(1),
			RunE:  runRegistryFetch,
		}

		registrySubmitCmd := &cobra.Command{
			Use:   "submit <file>",
			Short: "Validate and prepare a profile for community submission",
			Long:  `Reads a local profile YAML, validates its structure, and prints instructions for submitting it to the community registry via GitHub pull request.`,
			Args:  cobra.ExactArgs(1),
			RunE:  runRegistrySubmit,
		}

		registryCmd.AddCommand(registryListCmd, registrySearchCmd,
			registryFetchCmd, registrySubmitCmd)

		rootCmd.AddCommand(probeCmd, initCmd, versionCmd, configCmd,
			installCmd, removeCmd, listCmd, searchCmd, updateCmd, profileCmd,
			wslCmd, dotfilesCmd, modeCmd, containerCmd, ledgerCmd, teleportCmd,
			registryCmd)

		if err := rootCmd.ExecuteContext(ctx); err != nil {
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

