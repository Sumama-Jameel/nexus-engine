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

package manifest

// ResolveTarget determines which TargetConfig from the profile matches the
// detected package manager. It uses a priority system to find the best match:
//
//  1. Exact family match (e.g., "debian" for apt, "arch" for pacman)
//  2. "ubuntu" target as fallback for "debian" family (apt compatibility)
//  3. Generic "linux" target (cross-family fallback)
//  4. First target as last resort (ensures something is always returned)
//
// Returns nil only if the profile has no targets at all.
func ResolveTarget(profile *NexusProfile, packageManager string) *TargetConfig {
        // Map package manager to family
        pmToFamily := map[string]string{
                "apt":    "debian",
                "pacman": "arch",
                "dnf":    "fedora",
                "yum":    "fedora",
                "apk":    "alpine",
        }

        targetFamily := pmToFamily[packageManager]

        // Priority 1: Exact match
        for i := range profile.Targets {
                if profile.Targets[i].Family == targetFamily {
                        return &profile.Targets[i]
                }
        }

        // Priority 2: "ubuntu" maps to "debian" target
        if targetFamily == "debian" {
                for i := range profile.Targets {
                        if profile.Targets[i].Family == "ubuntu" {
                                return &profile.Targets[i]
                        }
                }
        }

        // Priority 3: Generic "linux" target
        for i := range profile.Targets {
                if profile.Targets[i].Family == "linux" {
                        return &profile.Targets[i]
                }
        }

        // Priority 4: First target
        if len(profile.Targets) > 0 {
                return &profile.Targets[0]
        }

        return nil
}

// ResolveTargetFamily maps a package manager name (e.g., "apt", "pacman") to
// its corresponding family identifier (e.g., "debian", "arch"). Returns the
// family string and true if the mapping exists, or an empty string and false
// if the package manager is not recognized.
func ResolveTargetFamily(packageManager string) (string, bool) {
        pmToFamily := map[string]string{
                "apt":    "debian",
                "pacman": "arch",
                "dnf":    "fedora",
                "yum":    "fedora",
                "apk":    "alpine",
        }
        f, ok := pmToFamily[packageManager]
        return f, ok
}

// CountPackages returns the total number of unique packages across all
// targets in the profile. This is used for progress reporting and UI display.
func CountPackages(profile *NexusProfile) int {
        total := 0
        for _, t := range profile.Targets {
                total += len(t.Packages)
        }
        return total
}

// Families returns the list of unique package family names referenced by the
// profile's targets. The order of families corresponds to their first
// appearance in the Targets slice.
func Families(profile *NexusProfile) []string {
        seen := make(map[string]bool)
        var families []string
        for _, t := range profile.Targets {
                if !seen[t.Family] {
                        seen[t.Family] = true
                        families = append(families, t.Family)
                }
        }
        return families
}

// NOTE: ApplyManifest was REMOVED in V3.
// The old ApplyManifest bypassed the Orchestrator's pre-flight checks,
// priority ordering, concurrent install, verify, rollback, audit, and
// state tracking. This is a CRITICAL security and reliability gap.
//
// The correct path is: Parse profile → ResolveTarget → pass packages
// to the Orchestrator, which handles the full flow:
// PreFlight → RefreshIndex → Order → Execute → Verify → Record → Audit → Report
//
// This is exactly what `nexus init` and `nexus install --profile` do.
