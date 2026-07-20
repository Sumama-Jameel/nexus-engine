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

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/spf13/cobra"
)

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
