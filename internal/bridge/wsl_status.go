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

package bridge

import (
	"fmt"
	"strings"
)

// WSL2Status captures the full WSL2 detection intelligence report.
// Per V4 "The WSL2 Detector (The Spy)":
// "A Windows .exe version of your Go tool that checks if WSL2 is enabled."
//
// This struct is the "intelligence report" from the Spy. It tells the user:
// - Is WSL2 installed? (WSLAvailable)
// - What version? (WSLVersion: "1" or "2")
// - What distributions exist? (Distros)
// - What's preventing Nexus setup? (Blockers)
// - What should the user do? (Recommendations)
//
// This struct is JSON-serializable for IPC (future Tauri HUD in V10).
type WSL2Status struct {
	WSLAvailable    bool        `json:"wsl_available"`    // Is WSL feature enabled at all?
	WSLVersion      string      `json:"wsl_version"`      // "1", "2", or "unknown"
	DefaultDistro   string      `json:"default_distro"`   // Default WSL distribution name
	Distros         []WSLDistro `json:"distros"`          // All installed distributions
	KernelVersion   string      `json:"kernel_version"`   // WSL2 kernel version (e.g., "5.15.133.1")
	HyperVAvailable bool        `json:"hyperv_available"` // Is Hyper-V present? (required for WSL2)
	WindowsVersion  string      `json:"windows_version"`  // "Windows 10 22H2" or "Windows 11"
	WindowsBuild    int         `json:"windows_build"`    // Build number (19041+ = WSL2 capable)
	Architecture    string      `json:"architecture"`     // "amd64" or "arm64"
	Ready           bool        `json:"ready"`            // Can Nexus be installed right now?
	Blockers        []string    `json:"blockers"`         // Specific issues preventing install
	Recommendations []string    `json:"recommendations"`  // What the user should do next
}

// WSLDistro represents a single installed WSL distribution, as reported by
// `wsl --list --verbose`. It captures the distribution name, running state,
// WSL version (1 or 2), and whether it is the default distribution.
type WSLDistro struct {
	Name    string `json:"name"`
	State   string `json:"state"`   // "Running", "Stopped"
	Version string `json:"version"` // "1", "2"
	Default bool   `json:"default"`
}

// FormatWSL2Status returns a detailed human-readable terminal output of the
// WSL2 detection report. It renders Windows version, WSL status, Hyper-V
// availability, installed distributions, readiness assessment, blockers,
// and recommendations in a formatted panel suitable for CLI display.
func FormatWSL2Status(status *WSL2Status) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("  ╔══════════════════════════════════════════════════╗\n")
	sb.WriteString("  ║        NEXUS PROTOCOL — WSL2 DETECTOR            ║\n")
	sb.WriteString("  ╚══════════════════════════════════════════════════╝\n")
	sb.WriteString("\n")

	// Windows version
	sb.WriteString("  ── WINDOWS ──────────────────────────────────────\n")
	if status.WindowsVersion != "" {
		sb.WriteString(fmt.Sprintf("  🖥️  Version:       %s\n", status.WindowsVersion))
	}
	if status.WindowsBuild > 0 {
		sb.WriteString(fmt.Sprintf("  🏗️  Build:         %d\n", status.WindowsBuild))
	}
	sb.WriteString(fmt.Sprintf("  💻 Architecture:  %s\n", status.Architecture))
	sb.WriteString("\n")

	// WSL Status
	sb.WriteString("  ── WSL STATUS ───────────────────────────────────\n")
	if status.WSLAvailable {
		sb.WriteString("  ✅ WSL:           Available\n")
	} else {
		sb.WriteString("  ❌ WSL:           Not Available\n")
	}

	if status.WSLVersion != "" {
		versionLabel := "WSL" + status.WSLVersion
		if status.WSLVersion == "2" {
			versionLabel += " ✅ (Recommended)"
		} else if status.WSLVersion == "1" {
			versionLabel += " ⚠️  (Upgrade to WSL2 recommended)"
		}
		sb.WriteString(fmt.Sprintf("  🔢 Version:       %s\n", versionLabel))
	}

	if status.KernelVersion != "" {
		sb.WriteString(fmt.Sprintf("  🧬 Kernel:        %s\n", status.KernelVersion))
	}

	if status.HyperVAvailable {
		sb.WriteString("  ✅ Hyper-V:       Available\n")
	} else {
		sb.WriteString("  ❌ Hyper-V:       Not Available (required for WSL2)\n")
	}

	// Default distro
	if status.DefaultDistro != "" {
		sb.WriteString(fmt.Sprintf("  🐧 Default Distro: %s\n", status.DefaultDistro))
	}

	// Distribution list
	if len(status.Distros) > 0 {
		sb.WriteString("\n  ── INSTALLED DISTRIBUTIONS ──────────────────────\n")
		sb.WriteString(fmt.Sprintf("  %-20s %-12s %-8s %s\n", "NAME", "STATE", "VERSION", "DEFAULT"))
		sb.WriteString("  " + strings.Repeat("─", 50) + "\n")
		for _, d := range status.Distros {
			defaultMark := ""
			if d.Default {
				defaultMark = "⭐"
			}
			sb.WriteString(fmt.Sprintf("  %-20s %-12s %-8s %s\n", d.Name, d.State, d.Version, defaultMark))
		}
	}

	// Readiness
	sb.WriteString("\n  ── NEXUS READINESS ──────────────────────────────\n")
	if status.Ready {
		sb.WriteString("  ✅ System is READY for Nexus setup\n")
	} else {
		sb.WriteString("  ⛔ System is NOT READY for Nexus setup\n")
	}

	if len(status.Blockers) > 0 {
		sb.WriteString("\n  ── BLOCKERS ─────────────────────────────────────\n")
		for _, b := range status.Blockers {
			sb.WriteString(fmt.Sprintf("  🔴 %s\n", b))
		}
	}

	if len(status.Recommendations) > 0 {
		sb.WriteString("\n  ── RECOMMENDATIONS ──────────────────────────────\n")
		for i, r := range status.Recommendations {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, r))
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// FormatWSL2Check returns a minimal pass/fail output for the `nexus wsl check`
// command. Unlike FormatWSL2Status which shows full details, this function
// outputs only the readiness result and any blockers.
func FormatWSL2Check(status *WSL2Status) string {
	var sb strings.Builder

	sb.WriteString("\n")
	if status.Ready {
		sb.WriteString("  ✅ READY — This system can run Nexus via WSL2\n")
	} else {
		sb.WriteString("  ⛔ NOT READY — This system cannot run Nexus via WSL2\n")
		if len(status.Blockers) > 0 {
			sb.WriteString("\n  Blockers:\n")
			for _, b := range status.Blockers {
				sb.WriteString(fmt.Sprintf("    • %s\n", b))
			}
		}
	}
	sb.WriteString("\n")

	return sb.String()
}
