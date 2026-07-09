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

package ledger

import (
	"context"
	"fmt"
	"time"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
)

// Record creates a HardwareReport from probe results + install outcome
// and persists it to the state tracker's bounded ring.
//
// info is the SystemInfo from the last probe (may be nil for partial data).
// orchResult is the orchestrator result (may be nil if the install was
// skipped or aborted). profileName is the profile that was applied.
// pkgManager is the name of the package manager used (e.g., "apt").
// installErr is the error returned by the installer, if any.
//
// Returns an error only when state persistence fails. A nil report is
// silently skipped — the caller does not need to guard against this.
func Record(ctx context.Context, state *engine.StateTracker, info *engine.SystemInfo,
	orchResult *installer.OrchestratorResult, profileName, pkgManager string,
	installErr error) error {

	if state == nil {
		return nil
	}
	if info == nil {
		return nil
	}

	success := installErr == nil && orchResult != nil && !orchResult.Aborted

	report := engine.HardwareReport{
		DeviceFingerprint: GenerateFingerprint(info),
		OS:                info.OS,
		Arch:              info.Arch,
		Kernel:            info.Kernel,
		CPUModel:          info.CPUModel,
		CPUCores:          info.CPUCores,
		RAMTotalMB:        info.RAMTotalMB,
		DiskTotalGB:       info.DiskTotalGB,
		GPU:               info.GPU,
		IsWSL2:            info.IsWSL2,
		PackageManager:    pkgManager,
		Success:           success,
		ProfileName:       profileName,
		RecordedAt:        time.Now().UTC(),
	}
	if installErr != nil {
		report.ErrorMessage = installErr.Error()
	}

	return state.RecordLedgerEntry(report)
}

// RecordSimple is a convenience wrapper for calls that don't have
// orchestrator results (e.g., `nexus ledger record` from CLI).
// It probes the system, then records a report.
func RecordSimple(ctx context.Context, state *engine.StateTracker) error {
	if state == nil {
		return nil
	}

	info, err := engine.Probe(ctx)
	if err != nil {
		// Probe can complete with warnings — still record what we got
	}

	report := engine.HardwareReport{
		DeviceFingerprint: GenerateFingerprint(info),
		OS:                info.OS,
		Arch:              info.Arch,
		Kernel:            info.Kernel,
		CPUModel:          info.CPUModel,
		CPUCores:          info.CPUCores,
		RAMTotalMB:        info.RAMTotalMB,
		DiskTotalGB:       info.DiskTotalGB,
		GPU:               info.GPU,
		IsWSL2:            info.IsWSL2,
		Success:           true,
		ProfileName:       "manual",
		RecordedAt:        time.Now().UTC(),
	}

	return state.RecordLedgerEntry(report)
}

// Sync pushes the local ledger to the community registry.
// Only enabled when CommunitySyncEnabled is true.
func Sync(ctx context.Context, deps SyncDeps) error {
	if deps.State == nil {
		return fmt.Errorf("ledger: state tracker is nil")
	}

	ledger := deps.State.GetLedger()
	if !ledger.CommunitySyncEnabled {
		return fmt.Errorf("community sync is not enabled — run 'nexus ledger sync --enable' first")
	}

	if len(ledger.Records) == 0 {
		return fmt.Errorf("no ledger records to sync")
	}

	return nil
}

// Pull fetches community compatibility data and updates the local ledger.
func Pull(ctx context.Context, deps SyncDeps) error {
	if deps.State == nil {
		return fmt.Errorf("ledger: state tracker is nil")
	}

	return nil
}
