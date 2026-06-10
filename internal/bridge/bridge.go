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
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// ExecFunc is the type signature for the centralized execution function.
// Per the Nexus Protocol Zero-Trust Architecture: "Every system call the
// Go engine makes to the shell must pass through a centralized
// SanitizeAndExecute function." The bridge package uses this type to
// delegate all command execution to the security gate.
//
// This is dependency injection — the bridge receives the execution
// function rather than calling exec.Command directly, ensuring no
// command bypasses sanitization, whitelisting, or timeout enforcement.
type ExecFunc func(ctx context.Context, command string, args ...string) (string, error)

// bridgeExecFn is the package-level execution function used by all
// bridge detection code. It defaults to a raw exec.Command wrapper
// (for standalone operation) but SHOULD be overridden via SetExecFunc
// to route through SanitizeAndExecute for Zero-Trust compliance.
var (
	bridgeExecFn ExecFunc
)

func init() {
        // Default: raw exec.Command wrapper (no security gate).
        // This allows the bridge package to function standalone
        // (e.g., in tests or non-Nexus contexts).
        // Production code MUST call SetExecFunc(engine.SanitizeAndExecute).
        bridgeExecFn = defaultExecFunc
}

// defaultExecFunc is the fallback execution function that uses
// exec.Command directly. This is only used when SetExecFunc has
// not been called — it provides backward compatibility but does
// NOT enforce the Zero-Trust security gate.
func defaultExecFunc(ctx context.Context, command string, args ...string) (string, error) {
        cmd := exec.CommandContext(ctx, command, args...)
        output, err := cmd.Output()
        if err != nil {
                return "", err
        }
        return string(output), nil
}

// SetExecFunc configures the bridge package to route ALL command
// execution through the provided security gate function.
//
// Per the Nexus Protocol: "Every system call the Go engine makes to
// the shell must pass through a centralized SanitizeAndExecute function."
// This function is the integration point — call it once at program
// startup with engine.SanitizeAndExecute to enforce Zero-Trust.
//
// Example:
//
//      bridge.SetExecFunc(engine.SanitizeAndExecute)
func SetExecFunc(fn ExecFunc) {
        if fn != nil {
                bridgeExecFn = fn
        }
}

// EnvironmentInfo describes the detected execution environment.
// This struct is platform-agnostic — it is populated by the
// platform-specific detectEnvironmentImpl() function.
//
// Per the Nexus Protocol:
// "Probe: Go queries the OS using runtime.GOOS and system calls
//  to detect if it's running natively on Linux or inside WSL2."
type EnvironmentInfo struct {
        // WSL2Status contains the full WSL2 detection report when running
        // on Windows. On Linux, this field is nil — use IsWSL2 instead
        // to check if running inside WSL2.
        //
        // Per V4 "The Spy": The Windows .exe detects WSL2 from the host
        // side. This field connects that intelligence to the main
        // environment detection flow, so `nexus probe` on Windows
        // shows WSL2 information without requiring a separate command.
        WSL2Status     *WSL2Status     `json:"wsl2_status,omitempty"`
        Prerequisites  map[string]bool `json:"prerequisites"`
        Blockers       []string        `json:"blockers"`
        WindowsVersion string          `json:"windows_version,omitempty"`
        Distro         string          `json:"distro"`
        PackageManager string          `json:"package_manager"`
        WindowsBuild   int             `json:"windows_build,omitempty"`
        IsWSL2         bool            `json:"is_wsl2"`
        IsNativeLinux  bool            `json:"is_native_linux"`
        IsWindows      bool            `json:"is_windows"`
        Ready          bool            `json:"ready"`
}

// DetectEnvironment probes the current OS environment and determines
// the execution context: WSL2, native Linux, or Windows.
//
// This function is the platform-agnostic entry point. It delegates
// to detectEnvironmentImpl() which is compiled per-platform via
// Go build tags (bridge_linux.go, bridge_windows.go).
//
// Per the Nexus Protocol Zero-Trust Architecture:
// "No component trusts another by default." The detection result
// is validated before being returned — prerequisites are checked
// and blockers are explicitly enumerated.
func DetectEnvironment(ctx context.Context) *EnvironmentInfo {
        env := detectEnvironmentImpl(ctx)

        // Validate prerequisites (cross-platform)
        env.Prerequisites = engine.ValidatePrerequisites(ctx)

        // Determine readiness and blockers
        if env.Blockers == nil {
                env.Blockers = []string{}
        }
        for tool, found := range env.Prerequisites {
                if !found {
                        env.Blockers = append(env.Blockers, tool+" is not installed")
                }
        }
        env.Ready = len(env.Blockers) == 0

        return env
}

// FormatEnvironmentInfo returns a human-readable summary of the environment.
// This formatter is platform-agnostic — it renders whatever EnvironmentInfo
// contains, regardless of which platform detected it.
func FormatEnvironmentInfo(env *EnvironmentInfo) string {
        var sb strings.Builder

        sb.WriteString("\n")
        sb.WriteString("  ── ENVIRONMENT DETECTION ───────────────────────\n")

        if env.IsWSL2 {
                sb.WriteString("  🪟 Mode:          WSL2 (Windows Subsystem for Linux)\n")
        } else if env.IsNativeLinux {
                sb.WriteString("  🐧 Mode:          Native Linux\n")
        } else if env.IsWindows {
                sb.WriteString("  🪟 Mode:          Windows (Native)\n")
                if env.WindowsVersion != "" {
                        sb.WriteString("  🖥️  Windows:      " + env.WindowsVersion + "\n")
                }
                if env.WindowsBuild > 0 {
                        sb.WriteString(fmt.Sprintf("  🏗️  Build:         %d\n", env.WindowsBuild))
                }
        }

        if env.Distro != "" && env.Distro != "unknown" {
                sb.WriteString("  📦 Distro:        " + env.Distro + "\n")
        }
        if env.PackageManager != "" && env.PackageManager != "unknown" {
                sb.WriteString("  🔧 Pkg Manager:   " + env.PackageManager + "\n")
        }

        // WSL2 Status summary (V4: The Spy)
        if env.WSL2Status != nil {
                sb.WriteString("\n  ── WSL2 STATUS (The Spy) ─────────────────────\n")
                if env.WSL2Status.WSLAvailable {
                        sb.WriteString("  ✅ WSL:           Available\n")
                } else {
                        sb.WriteString("  ❌ WSL:           Not Available\n")
                }
                if env.WSL2Status.WSLVersion != "" {
                        versionLabel := "WSL" + env.WSL2Status.WSLVersion
                        if env.WSL2Status.WSLVersion == "2" {
                                versionLabel += " ✅"
                        } else if env.WSL2Status.WSLVersion == "1" {
                                versionLabel += " ⚠️"
                        }
                        sb.WriteString(fmt.Sprintf("  🔢 Version:       %s\n", versionLabel))
                }
                if env.WSL2Status.HyperVAvailable {
                        sb.WriteString("  ✅ Hyper-V:       Available\n")
                } else {
                        sb.WriteString("  ❌ Hyper-V:       Not Available\n")
                }
                if len(env.WSL2Status.Distros) > 0 {
                        sb.WriteString(fmt.Sprintf("  🐧 Distros:       %d installed\n", len(env.WSL2Status.Distros)))
                        if env.WSL2Status.DefaultDistro != "" {
                                sb.WriteString("  ⭐ Default:       " + env.WSL2Status.DefaultDistro + "\n")
                        }
                }
                if env.WSL2Status.Ready {
                        sb.WriteString("  ✅ WSL2 Readiness: READY\n")
                } else {
                        sb.WriteString("  ⛔ WSL2 Readiness: NOT READY\n")
                }
        }

        sb.WriteString("\n  ── PREREQUISITES ───────────────────────────────\n")
        for tool, found := range env.Prerequisites {
                status := "❌ MISSING"
                if found {
                        status = "✅ Found"
                }
                sb.WriteString("  " + tool + ": " + status + "\n")
        }

        if env.Ready {
                sb.WriteString("\n  ✅ Environment is READY for nexus init\n")
        } else {
                sb.WriteString("\n  ⛔ Environment NOT READY — resolve blockers:\n")
                for _, blocker := range env.Blockers {
                        sb.WriteString("    • " + blocker + "\n")
                }
        }

        return sb.String()
}
