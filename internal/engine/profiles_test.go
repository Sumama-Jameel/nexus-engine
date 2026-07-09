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

package engine

import (
	"strings"
	"testing"

	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
)

func TestCheckDistroCompatibility_EmptySuggested(t *testing.T) {
	system := &SystemInfo{DistroID: "ubuntu", DistroVersion: "24.04", IsWSL2: false}
	comp := CheckDistroCompatibility("test-profile", "", system)
	if comp.Match != DistroExact {
		t.Fatalf("expected DistroExact for empty suggested_distro, got %v", comp.Match)
	}
	if comp.Message == "" {
		t.Fatal("expected non-empty message")
	}
}

func TestCheckDistroCompatibility_Any(t *testing.T) {
	system := &SystemInfo{DistroID: "ubuntu", DistroVersion: "24.04", IsWSL2: false}
	comp := CheckDistroCompatibility("test-profile", "any", system)
	if comp.Match != DistroExact {
		t.Fatalf("expected DistroExact for 'any', got %v", comp.Match)
	}
}

func TestCheckDistroCompatibility_ExactMatch(t *testing.T) {
	system := &SystemInfo{DistroID: "pop", DistroVersion: "24.04", IsWSL2: false}
	comp := CheckDistroCompatibility("gamer", "pop", system)
	if comp.Match != DistroExact {
		t.Fatalf("expected DistroExact, got %v (msg: %s)", comp.Match, comp.Message)
	}
}

func TestCheckDistroCompatibility_WSL2Mismatch(t *testing.T) {
	system := &SystemInfo{DistroID: "ubuntu", DistroVersion: "24.04", IsWSL2: true}
	comp := CheckDistroCompatibility("gamer", "pop", system)
	if comp.Match != DistroContainerFallback {
		t.Fatalf("expected DistroContainerFallback (pop not in WSL2 image list), got %v (msg: %s)", comp.Match, comp.Message)
	}
	if !comp.IsWSL2 {
		t.Fatal("expected IsWSL2=true")
	}
	if comp.WSL2Available {
		t.Fatal("expected WSL2Available=false for pop-os")
	}
}

func TestCheckDistroCompatibility_WSL2MismatchKali(t *testing.T) {
	system := &SystemInfo{DistroID: "ubuntu", DistroVersion: "24.04", IsWSL2: true}
	comp := CheckDistroCompatibility("hacker", "kali", system)
	if comp.Match != DistroWSL2Switch {
		t.Fatalf("expected DistroWSL2Switch (kali is in WSL2 image list), got %v (msg: %s)", comp.Match, comp.Message)
	}
	if !comp.WSL2Available {
		t.Fatal("expected WSL2Available=true for kali")
	}
}

func TestCheckDistroCompatibility_NativeMismatch(t *testing.T) {
	system := &SystemInfo{DistroID: "ubuntu", DistroVersion: "24.04", IsWSL2: false}
	comp := CheckDistroCompatibility("gamer", "pop", system)
	if comp.Match != DistroContainerFallback {
		t.Fatalf("expected DistroContainerFallback, got %v (msg: %s)", comp.Match, comp.Message)
	}
	if comp.IsWSL2 {
		t.Fatal("expected IsWSL2=false on native linux")
	}
}

func TestCheckDistroCompatibility_UnknownDistro(t *testing.T) {
	system := &SystemInfo{DistroID: "", DistroVersion: "", IsWSL2: false}
	comp := CheckDistroCompatibility("test", "ubuntu", system)
	if comp.Match != DistroExact {
		t.Fatalf("expected DistroExact when current distro unknown, got %v", comp.Match)
	}
}

func TestDistroFamily_Mapping(t *testing.T) {
	tests := []struct {
		id     string
		family string
	}{
		{"ubuntu", "debian"},
		{"debian", "debian"},
		{"kali", "debian"},
		{"pop", "debian"},
		{"linuxmint", "debian"},
		{"arch", "arch"},
		{"manjaro", "arch"},
		{"garuda", "arch"},
		{"fedora", "fedora"},
		{"rhel", "fedora"},
		{"centos", "fedora"},
		{"rocky", "fedora"},
		{"alpine", "alpine"},
		{"opensuse", "suse"},
		{"void", "void"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := distroFamily(tt.id)
		if got != tt.family {
			t.Errorf("distroFamily(%q) = %q, want %q", tt.id, got, tt.family)
		}
	}
}

func TestIsSameFamily(t *testing.T) {
	if !isSameFamily("ubuntu", "debian") {
		t.Error("expected ubuntu and debian to be same family")
	}
	if !isSameFamily("kali", "ubuntu") {
		t.Error("expected kali and ubuntu to be same family")
	}
	if !isSameFamily("pop", "ubuntu") {
		t.Error("expected pop and ubuntu to be same family")
	}
	if isSameFamily("ubuntu", "arch") {
		t.Error("expected ubuntu and arch to NOT be same family")
	}
	if isSameFamily("fedora", "arch") {
		t.Error("expected fedora and arch to NOT be same family")
	}
	if isSameFamily("", "ubuntu") {
		t.Error("expected empty string to NOT match any family")
	}
}

func TestScoreProfile_NvidiaBonus(t *testing.T) {
	system := &SystemInfo{GPU: "NVIDIA Corporation GA102 [GeForce RTX 3080]"}
	comp := DistroCompatibility{Match: DistroExact}
	// A profile named "gamer-nvidia" should get bonus
	score := scoreProfile(manifest.NexusProfile{Name: "base-gamer-nvidia"}, system, comp)
	if score <= 100 {
		t.Errorf("expected score > 100 (exact match + nvidia bonus), got %d", score)
	}
}

func TestScoreProfile_NoGpuBonus(t *testing.T) {
	system := &SystemInfo{GPU: "N/A"}
	comp := DistroCompatibility{Match: DistroExact}
	score := scoreProfile(manifest.NexusProfile{Name: "base-gamer"}, system, comp)
	if score != 100 {
		t.Errorf("expected score 100 (exact match, no bonus), got %d", score)
	}
}

func TestFormatProfileSuggestions_Empty(t *testing.T) {
	s := FormatProfileSuggestions(nil)
	if len(s) == 0 {
		t.Fatal("expected non-empty string for nil suggestions")
	}
}

func TestFormatProfileSuggestions_Single(t *testing.T) {
	suggestions := []ProfileSuggestion{
		{Name: "test", Score: 100, Compatibility: DistroCompatibility{Message: "perfect match"}},
	}
	s := FormatProfileSuggestions(suggestions)
	if !strings.Contains(s, "test") {
		t.Fatalf("expected output to contain 'test':\n%s", s)
	}
}

func TestSuggestProfileLoadsBundledDefaults(t *testing.T) {
	system := &SystemInfo{DistroID: "ubuntu", DistroVersion: "24.04", IsWSL2: false}
	suggestions := SuggestProfile(system, nil)
	if len(suggestions) == 0 {
		t.Fatal("expected at least some bundled defaults to be suggested")
	}
	// The highest-scoring suggestion should be ubuntu-friendly (base-dev, base-work, etc.)
	best := suggestions[0]
	if best.Score < 70 {
		t.Errorf("expected best score >= 70 for ubuntu, got %d (%s)", best.Score, best.Name)
	}
}

func TestProbeDistro_ParsesOsRelease(t *testing.T) {
	info := &SystemInfo{}
	err := probeDistro(info)
	if err != nil {
		t.Fatalf("probeDistro should not error: %v", err)
	}
	// On this system, it should detect something (probably Ubuntu)
	if info.DistroID == "" {
		t.Log("DistroID empty — expected in CI/container without /etc/os-release")
	} else {
		t.Logf("Detected: %s %s", info.DistroID, info.DistroVersion)
	}
}
