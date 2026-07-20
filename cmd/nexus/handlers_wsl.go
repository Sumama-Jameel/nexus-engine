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
	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/wsl"
	"github.com/spf13/cobra"
)

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
