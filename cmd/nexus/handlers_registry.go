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
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
	"github.com/spf13/cobra"
)

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
