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
	"github.com/Sumama-Jameel/nexus-engine/internal/ledger"
	"github.com/spf13/cobra"
)

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
