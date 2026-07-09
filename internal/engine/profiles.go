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
	"fmt"
	"sort"
	"strings"

	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
)

// DistroMatch describes how well a profile's suggested distro matches the
// user's current distro and what action to take.
type DistroMatch int

const (
	DistroExact DistroMatch = iota
	DistroFamily
	DistroWSL2Switch
	DistroContainerFallback
)

func (m DistroMatch) String() string {
	switch m {
	case DistroExact:
		return "exact"
	case DistroFamily:
		return "family"
	case DistroWSL2Switch:
		return "wsl2"
	case DistroContainerFallback:
		return "container"
	default:
		return "unknown"
	}
}

// DistroCompatibility holds the result of checking a profile against the
// current system's distribution.
type DistroCompatibility struct {
	ProfileName     string      `json:"profile_name"`
	SuggestedDistro string      `json:"suggested_distro"`
	CurrentDistro   string      `json:"current_distro"`
	Match           DistroMatch `json:"match"`
	IsWSL2          bool        `json:"is_wsl2"`
	WSL2Available   bool        `json:"wsl2_available"`
	Message         string      `json:"message"`
}

// ProfileSuggestion is a profile recommendation returned by SuggestProfile.
type ProfileSuggestion struct {
	Name        string             `json:"name"`
	Version     string             `json:"version"`
	Description string             `json:"description"`
	Author      string             `json:"author"`
	Compatibility DistroCompatibility `json:"compatibility"`
	Score       int                `json:"score"`
}

// CheckDistroCompatibility evaluates how well a profile fits the current system.
// It reads the profile's suggested_distro field and compares it to the system's
// detected distro, returning a DistroCompatibility with a human-readable message.
// Three outcomes:
//   - DistroExact: the current distro matches the suggested one (or suggestion is "any"/empty)
//   - DistroWSL2Switch: on WSL2 — can reimport a different distro
//   - DistroContainerFallback: on native Linux — use Distrobox or install equivalent packages
func CheckDistroCompatibility(profileName, suggestedDistro string, system *SystemInfo) DistroCompatibility {
	if suggestedDistro == "" || suggestedDistro == "any" {
		return DistroCompatibility{
			ProfileName:     profileName,
			SuggestedDistro: suggestedDistro,
			CurrentDistro:   system.DistroID,
			Match:           DistroExact,
			IsWSL2:          system.IsWSL2,
			WSL2Available:   true,
			Message:         "Works on any Linux distribution — applying directly.",
		}
	}

	if system.DistroID == "" {
		return DistroCompatibility{
			ProfileName:     profileName,
			SuggestedDistro: suggestedDistro,
			CurrentDistro:   system.DistroID,
			Match:           DistroExact,
			IsWSL2:          system.IsWSL2,
			WSL2Available:   true,
			Message:         fmt.Sprintf("Designed for %s. Installing equivalent packages.", suggestedDistro),
		}
	}

	// Exact match
	if system.DistroID == suggestedDistro {
		return DistroCompatibility{
			ProfileName:     profileName,
			SuggestedDistro: suggestedDistro,
			CurrentDistro:   system.DistroID,
			Match:           DistroExact,
			IsWSL2:          system.IsWSL2,
			WSL2Available:   true,
			Message:         fmt.Sprintf("Perfect match! %s is the recommended distro for this profile.", suggestedDistro),
		}
	}

	// Mismatch — determine the path
	if system.IsWSL2 {
		wslAvailable := suggestedDistro == "ubuntu" || suggestedDistro == "kali" || suggestedDistro == "fedora" || suggestedDistro == "debian" || suggestedDistro == "arch"
		if wslAvailable {
			msg := fmt.Sprintf("Designed for %s. You're on %s WSL2.", suggestedDistro, system.DistroID)
			msg += fmt.Sprintf(" Run 'nexus wsl import %s' to switch, or install equivalent packages on current distro.", suggestedDistro)
			return DistroCompatibility{
				ProfileName:     profileName,
				SuggestedDistro: suggestedDistro,
				CurrentDistro:   system.DistroID,
				Match:           DistroWSL2Switch,
				IsWSL2:          true,
				WSL2Available:   true,
				Message:         msg,
			}
		}
		msg := fmt.Sprintf("Designed for %s. You're on %s WSL2.", suggestedDistro, system.DistroID)
		msg += " This distro is not available as a WSL2 image. Use Distrobox or install equivalent packages."
		return DistroCompatibility{
			ProfileName:     profileName,
			SuggestedDistro: suggestedDistro,
			CurrentDistro:   system.DistroID,
			Match:           DistroContainerFallback,
			IsWSL2:          true,
			WSL2Available:   false,
			Message:         msg,
		}
	}

	// Native Linux — Distrobox fallback
	msg := fmt.Sprintf("Designed for %s. You're on %s.", suggestedDistro, system.DistroID)
	msg += " Installing equivalent packages now. For the full experience, use Distrobox: 'distrobox create --image " + suggestedDistro + ":latest'."
	return DistroCompatibility{
		ProfileName:     profileName,
		SuggestedDistro: suggestedDistro,
		CurrentDistro:   system.DistroID,
		Match:           DistroContainerFallback,
		IsWSL2:          false,
		WSL2Available:   false,
		Message:         msg,
	}
}

// SuggestProfile recommends profiles from the bundled defaults based on the
// user's detected system hardware and distro. It scores each profile by how
// well it matches the current system and returns them sorted by score.
func SuggestProfile(system *SystemInfo, profiles []manifest.NexusProfile) []ProfileSuggestion {
	if len(profiles) == 0 {
		// Load from bundled defaults
		bundled := manifest.BundledDefaults()
		for name, content := range bundled {
			parsed, err := manifest.ParseBytes([]byte(content))
			if err != nil {
				continue
			}
			parsed.Name = name
			profiles = append(profiles, *parsed)
		}
	}

	suggestions := make([]ProfileSuggestion, 0, len(profiles))
	for _, p := range profiles {
		name := p.Name
		comp := CheckDistroCompatibility(name, p.SuggestedDistro, system)
		score := scoreProfile(p, system, comp)
		suggestions = append(suggestions, ProfileSuggestion{
			Name:          name,
			Version:       p.Version,
			Description:   p.Description,
			Author:        p.Author,
			Compatibility: comp,
			Score:         score,
		})
	}

	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Score > suggestions[j].Score
	})
	return suggestions
}

// scoreProfile assigns a numeric score for ranking profile suggestions.
// Higher is better. Exact distro match = 100, family match = 70,
// WSL2 switchable = 50, container = 30, unknown = 10.
func scoreProfile(p manifest.NexusProfile, system *SystemInfo, comp DistroCompatibility) int {
	score := 0

	// Distro match
	switch comp.Match {
	case DistroExact:
		score += 100
	case DistroFamily:
		score += 70
	case DistroWSL2Switch:
		score += 50
	case DistroContainerFallback:
		score += 30
	}

	// Bonus for matching GPU vendor
	gpu := strings.ToLower(system.GPU)
	if strings.Contains(gpu, "nvidia") && strings.Contains(p.Name, "nvidia") {
		score += 30
	}
	if strings.Contains(gpu, "amd") && strings.Contains(p.Name, "amd") {
		score += 30
	}
	if strings.Contains(gpu, "intel") && p.SuggestedDistro == "any" {
		score += 10
	}

	// Bonus for profiles that extend the detected base
	if system.DistroID == "ubuntu" && p.SuggestedDistro == "ubuntu" {
		score += 20
	}

	return score
}

func isSameFamily(a, b string) bool {
	return distroFamily(a) == distroFamily(b) && distroFamily(a) != ""
}

func distroFamily(id string) string {
	switch id {
	case "ubuntu", "debian", "kali", "pop", "linuxmint", "elementary", "zorin":
		return "debian"
	case "arch", "manjaro", "endeavouros", "garuda":
		return "arch"
	case "fedora", "rhel", "centos", "rocky", "alma":
		return "fedora"
	case "alpine":
		return "alpine"
	case "opensuse", "suse":
		return "suse"
	case "void":
		return "void"
	default:
		return ""
	}
}

// FormatProfileSuggestions returns a human-readable table of profile suggestions.
func FormatProfileSuggestions(suggestions []ProfileSuggestion) string {
	if len(suggestions) == 0 {
		return "  No profile suggestions available.\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "  %-24s %-8s %s\n", "PROFILE", "SCORE", "DETAILS")
	fmt.Fprintf(&b, "  %s\n", strings.Repeat("─", 90))

	for _, s := range suggestions {
		scoreStr := fmt.Sprintf("%d", s.Score)
		if s.Score >= 100 {
			scoreStr = "⭐⭐"
		} else if s.Score >= 70 {
			scoreStr = "⭐"
		}
		desc := s.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		compat := s.Compatibility.Message
		if len(compat) > 40 {
			compat = compat[:37] + "..."
		}
		fmt.Fprintf(&b, "  %-24s %-8s %s\n", s.Name, scoreStr, desc)
	}
	fmt.Fprintf(&b, "\n")
	return b.String()
}
