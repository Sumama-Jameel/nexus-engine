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
	"fmt"
	"os"
	"strings"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
	"github.com/spf13/cobra"
)

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
