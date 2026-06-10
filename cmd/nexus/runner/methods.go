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

package runner

import (
	"context"
	"fmt"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
)

// ResolveProfilePackages loads a profile by name, resolves its extends chain,
// and returns the package list matching the current environment.
//
// Returns (nil, nil, error) if the profile cannot be loaded or resolved.
// Returns (profile, nil, nil) if the profile has no matching target.
func (d *Dependencies) ResolveProfilePackages(name string) (*manifest.NexusProfile, []string, error) {
	if d.ProfileStore == nil {
		return nil, nil, fmt.Errorf("profile store not available")
	}

	profile, err := d.ProfileStore.LoadProfileWithExtends(name)
	if err != nil {
		return nil, nil, err
	}

	target := manifest.ResolveTarget(profile, d.Env.PackageManager)
	if target == nil {
		return profile, nil, nil
	}

	return profile, target.Packages, nil
}

// InstallPackages orchestrates the full installation flow for the given packages.
// It delegates to the Orchestrator which handles: PreFlight → RefreshIndex →
// Order → Execute → Verify → RecordState → Audit → Report.
//
// The profileName is used for audit logging and state tracking.
func (d *Dependencies) InstallPackages(ctx context.Context, packages []string, profileName string) (*installer.OrchestratorResult, *manifest.NexusProfile, error) {
	orch := installer.NewOrchestrator(d.PM, d.ExecFn, d.State, d.Audit, profileName, d.DryRun)
	result, err := orch.Install(ctx, packages)
	return result, nil, err
}

// RemovePackages removes Nexus-managed packages with dependency warnings.
// It partitions the input into managed (removable) and unmanaged (skipped)
// packages, computes dependency warnings, then removes the managed ones.
func (d *Dependencies) RemovePackages(ctx context.Context, packages []string) (*RemoveResult, error) {
	result := &RemoveResult{}

	managed := d.State.GetManagedPackages()
	depMap := installer.BuildDependencyMap(managed)

	for _, pkg := range packages {
		if !d.State.IsManaged(pkg) {
			result.NotManaged = append(result.NotManaged, pkg)
			continue
		}
		result.ToRemove = append(result.ToRemove, pkg)

		if deps, ok := depMap[pkg]; ok {
			for _, dep := range deps {
				result.DependencyWarnings = append(result.DependencyWarnings,
					fmt.Sprintf("removing '%s' may affect dependent package '%s'", pkg, dep))
			}
		}
	}

	if len(result.ToRemove) == 0 {
		return result, nil
	}

	if d.DryRun {
		for _, pkg := range result.ToRemove {
			result.PackageResults = append(result.PackageResults, installer.PackageResult{
					Package:    pkg,
					Action:     "remove",
					Success:    true,
					Skipped:    true,
					SkipReason: "dry-run",
				})
		}
		return result, nil
	}

	removeResults, err := d.PM.Remove(ctx, result.ToRemove)
	if err != nil {
		return result, err
	}

	for _, pr := range removeResults {
		if pr.Success {
			_ = d.State.RecordRemove(pr.Package)
			d.logAudit(engine.AuditEntry{
				Action:         "remove",
				Package:        pr.Package,
				Result:         "success",
				PackageManager: d.PM.Name(),
			})
		} else {
			d.logAudit(engine.AuditEntry{
				Action:         "remove",
				Package:        pr.Package,
				Result:         "failure",
				PackageManager: d.PM.Name(),
				Error:          pr.Error,
			})
		}
	}

	result.PackageResults = removeResults
	return result, nil
}

// ListManagedPackages returns all packages tracked by Nexus state.
func (d *Dependencies) ListManagedPackages() (map[string]engine.PackageState, error) {
	return d.State.GetManagedPackages(), nil
}

// SearchPackages delegates to the package manager's search capability.
func (d *Dependencies) SearchPackages(ctx context.Context, query string) ([]string, error) {
	return d.PM.Search(ctx, query)
}

// UpdatePackages updates the given packages (or all if empty).
// Each package result is audit-logged for traceability.
func (d *Dependencies) UpdatePackages(ctx context.Context, packages []string) (*UpdateResult, error) {
	result := &UpdateResult{}

	if d.DryRun {
		for _, pkg := range packages {
			result.Packages = append(result.Packages, pkg)
			result.PackageResults = append(result.PackageResults, installer.PackageResult{
				Package:    pkg,
				Action:     "update",
				Success:    true,
				Skipped:    true,
				SkipReason: "dry-run",
			})
		}
		return result, nil
	}

	updateResults, err := d.PM.Update(ctx, packages)
	if err != nil {
		return result, err
	}

	for _, pr := range updateResults {
		if pr.Success {
			result.Packages = append(result.Packages, pr.Package)
		}
		d.logAudit(engine.AuditEntry{
			Action:         "update",
			Package:        pr.Package,
			Result:         boolToResult(pr.Success),
			PackageManager: d.PM.Name(),
			Error:          pr.Error,
		})
	}

	result.PackageResults = updateResults
	return result, nil
}

// ValidateProfileBytes validates raw YAML against the Nexus JSON Schema
// and Go-level semantic rules. This is a read-only operation with no side effects.
func (d *Dependencies) ValidateProfileBytes(data []byte) (*manifest.NexusProfile, error) {
	return manifest.ParseBytes(data)
}

// ApplyProfile loads a profile, resolves its target for the current environment,
// installs its packages, and records the profile as applied.
// Returns the orchestrator result and the loaded profile for display purposes.
func (d *Dependencies) ApplyProfile(ctx context.Context, name string) (*installer.OrchestratorResult, *manifest.NexusProfile, error) {
	if d.ProfileStore == nil {
		return nil, nil, fmt.Errorf("profile store not available")
	}

	profile, err := d.ProfileStore.LoadProfileWithExtends(name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load profile '%s': %w", name, err)
	}

	target := manifest.ResolveTarget(profile, d.Env.PackageManager)
	if target == nil {
		return nil, profile, fmt.Errorf("no compatible target found for package manager '%s' in profile '%s'", d.PM.Name(), name)
	}

	result, _, err := d.InstallPackages(ctx, target.Packages, name)
	if err != nil {
		return result, profile, err
	}

	// Record profile as applied (non-fatal)
	_ = d.ProfileStore.RecordApplied(name)

	return result, profile, nil
}

func boolToResult(b bool) string {
	if b {
		return "success"
	}
	return "failure"
}

// logAudit writes an audit entry if the AuditLogger is available.
// Matches the nil-safe pattern used by the Orchestrator (orchestrator.go:469).
// Audit logging is observability, not correctness — skipping it never
// changes the operation's outcome.
func (d *Dependencies) logAudit(entry engine.AuditEntry) {
	if d.Audit != nil {
		_ = d.Audit.Log(entry)
	}
}
