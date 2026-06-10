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

import "testing"

// ---------------------------------------------------------------------------
// ResolveTarget
// ---------------------------------------------------------------------------

func TestResolveTarget_ExactMatch(t *testing.T) {
	tests := []struct {
		name string
		pm   string
		family string
	}{
		{name: "apt maps to debian", pm: "apt", family: "debian"},
		{name: "pacman maps to arch", pm: "pacman", family: "arch"},
		{name: "dnf maps to fedora", pm: "dnf", family: "fedora"},
		{name: "yum maps to fedora", pm: "yum", family: "fedora"},
		{name: "apk maps to alpine", pm: "apk", family: "alpine"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			profile := &NexusProfile{
				Name:    "test",
				Version: "1.0.0",
				Targets: []TargetConfig{
					{Family: "debian", Packages: []string{"git"}},
					{Family: "arch", Packages: []string{"vim"}},
					{Family: "fedora", Packages: []string{"nano"}},
					{Family: "alpine", Packages: []string{"musl"}},
				},
			}

			result := ResolveTarget(profile, tc.pm)
			if result == nil {
				t.Fatalf("expected non-nil result for pm=%q", tc.pm)
			}
			if result.Family != tc.family {
				t.Errorf("expected family %q for pm=%q, got %q", tc.family, tc.pm, result.Family)
			}
		})
	}
}

func TestResolveTarget_UbuntuFallback(t *testing.T) {
	t.Parallel()

	// Profile has an "ubuntu" target but no "debian" target
	// apt should fall back to the ubuntu target (Priority 2)
	profile := &NexusProfile{
		Name:    "test",
		Version: "1.0.0",
		Targets: []TargetConfig{
			{Family: "ubuntu", Packages: []string{"git"}},
			{Family: "arch", Packages: []string{"vim"}},
		},
	}

	result := ResolveTarget(profile, "apt")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Family != "ubuntu" {
		t.Errorf("apt should fall back to 'ubuntu' target when no 'debian' exists, got %q", result.Family)
	}
}

func TestResolveTarget_GenericLinux(t *testing.T) {
	t.Parallel()

	// Profile has no debian or ubuntu target, but has a "linux" target
	profile := &NexusProfile{
		Name:    "test",
		Version: "1.0.0",
		Targets: []TargetConfig{
			{Family: "linux", Packages: []string{"git", "curl"}},
			{Family: "arch", Packages: []string{"vim"}},
		},
	}

	result := ResolveTarget(profile, "apt")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Family != "linux" {
		t.Errorf("apt should fall back to 'linux' generic target, got %q", result.Family)
	}
}

func TestResolveTarget_FirstTargetFallback(t *testing.T) {
	t.Parallel()

	// No matching family at all — should use first target as last resort
	profile := &NexusProfile{
		Name:    "test",
		Version: "1.0.0",
		Targets: []TargetConfig{
			{Family: "arch", Packages: []string{"vim"}},
			{Family: "fedora", Packages: []string{"nano"}},
		},
	}

	// "apt" → debian family, but no debian/ubuntu/linux target exists
	result := ResolveTarget(profile, "apt")
	if result == nil {
		t.Fatal("expected non-nil result (first target fallback)")
	}
	if result.Family != "arch" {
		t.Errorf("should fall back to first target 'arch', got %q", result.Family)
	}
}

func TestResolveTarget_EmptyTargets(t *testing.T) {
	t.Parallel()

	profile := &NexusProfile{
		Name:    "test",
		Version: "1.0.0",
		Targets: []TargetConfig{},
	}

	result := ResolveTarget(profile, "apt")
	if result != nil {
		t.Errorf("expected nil for empty targets, got %+v", result)
	}
}

// ---------------------------------------------------------------------------
// ResolveTargetFamily
// ---------------------------------------------------------------------------

func TestResolveTargetFamily(t *testing.T) {
	tests := []struct {
		pm      string
		family  string
		ok      bool
	}{
		{pm: "apt", family: "debian", ok: true},
		{pm: "pacman", family: "arch", ok: true},
		{pm: "dnf", family: "fedora", ok: true},
		{pm: "yum", family: "fedora", ok: true},
		{pm: "apk", family: "alpine", ok: true},
	}

	for _, tc := range tests {
		t.Run(tc.pm, func(t *testing.T) {
			t.Parallel()
			family, ok := ResolveTargetFamily(tc.pm)
			if ok != tc.ok {
				t.Errorf("ResolveTargetFamily(%q) ok = %v, expected %v", tc.pm, ok, tc.ok)
			}
			if family != tc.family {
				t.Errorf("ResolveTargetFamily(%q) = %q, expected %q", tc.pm, family, tc.family)
			}
		})
	}
}

func TestResolveTargetFamily_Unknown(t *testing.T) {
	t.Parallel()

	family, ok := ResolveTargetFamily("unknown-pm")
	if ok {
		t.Error("expected false for unknown package manager")
	}
	if family != "" {
		t.Errorf("expected empty family for unknown PM, got %q", family)
	}
}

// ---------------------------------------------------------------------------
// CountPackages
// ---------------------------------------------------------------------------

func TestCountPackages(t *testing.T) {
	t.Parallel()

	profile := &NexusProfile{
		Name:    "test",
		Version: "1.0.0",
		Targets: []TargetConfig{
			{Family: "debian", Packages: []string{"git", "curl", "wget"}},
			{Family: "arch", Packages: []string{"vim", "nano"}},
		},
	}

	count := CountPackages(profile)
	// 3 + 2 = 5
	if count != 5 {
		t.Errorf("expected 5 packages, got %d", count)
	}
}

func TestCountPackages_EmptyProfile(t *testing.T) {
	t.Parallel()

	profile := &NexusProfile{
		Name:    "test",
		Version: "1.0.0",
		Targets: []TargetConfig{},
	}

	count := CountPackages(profile)
	if count != 0 {
		t.Errorf("expected 0 packages for empty profile, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Families
// ---------------------------------------------------------------------------

func TestFamilies(t *testing.T) {
	t.Parallel()

	profile := &NexusProfile{
		Name:    "test",
		Version: "1.0.0",
		Targets: []TargetConfig{
			{Family: "debian", Packages: []string{"git"}},
			{Family: "arch", Packages: []string{"vim"}},
			{Family: "alpine", Packages: []string{"musl"}},
		},
	}

	families := Families(profile)
	if len(families) != 3 {
		t.Fatalf("expected 3 families, got %d", len(families))
	}

	// Order should match first appearance
	expected := []string{"debian", "arch", "alpine"}
	for i, f := range expected {
		if families[i] != f {
			t.Errorf("families[%d] = %q, expected %q", i, families[i], f)
		}
	}
}

func TestFamilies_EmptyProfile(t *testing.T) {
	t.Parallel()

	profile := &NexusProfile{
		Name:    "test",
		Version: "1.0.0",
		Targets: []TargetConfig{},
	}

	families := Families(profile)
	if len(families) != 0 {
		t.Errorf("expected 0 families for empty profile, got %d", len(families))
	}
}

func TestFamilies_Deduplication(t *testing.T) {
	t.Parallel()

	// Duplicate families should only appear once
	profile := &NexusProfile{
		Name:    "test",
		Version: "1.0.0",
		Targets: []TargetConfig{
			{Family: "debian", Packages: []string{"git"}},
			{Family: "debian", Packages: []string{"curl"}},
			{Family: "arch", Packages: []string{"vim"}},
		},
	}

	families := Families(profile)
	if len(families) != 2 {
		t.Fatalf("expected 2 unique families (deduplication), got %d: %v", len(families), families)
	}

	// Should preserve first-appearance order
	if families[0] != "debian" {
		t.Errorf("families[0] = %q, expected 'debian'", families[0])
	}
	if families[1] != "arch" {
		t.Errorf("families[1] = %q, expected 'arch'", families[1])
	}
}
