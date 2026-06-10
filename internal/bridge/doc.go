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

// Package bridge provides platform-aware environment detection and secure
// command execution for the Nexus Protocol engine.
//
// # Architecture
//
// The bridge package serves as the interface between the Nexus engine and the
// operating system. It has two primary responsibilities:
//
//  1. Environment Detection — Probe the OS to determine the execution context:
//     native Linux, WSL2, or Windows. This drives all downstream decisions
//     about package managers, install paths, and feature availability.
//
//  2. Command Execution Bridge — All shell commands issued by the engine must
//     pass through a centralized ExecFunc, which is injected at startup. This
//     enforces the Nexus Protocol Zero-Trust Architecture: "Every system call
//     the Go engine makes to the shell must pass through a centralized
//     SanitizeAndExecute function."
//
// # Platform Detection (Build Tags)
//
// Platform-specific detection is implemented via Go build tags:
//
//   - bridge_linux.go   — Detects WSL2 (via /proc/version), distro, package manager
//   - bridge_windows.go — Detects Windows version, WSL2 availability (the "Spy"),
//     Hyper-V, Windows package managers
//
// The DetectEnvironment function is the platform-agnostic entry point. It
// delegates to detectEnvironmentImpl(), which is compiled per-platform.
//
// # V4 "The Spy" — WSL2 Detection
//
// On Windows, the bridge includes the WSL2 detection intelligence ("The Spy")
// from V4. The WSL2Status struct captures the full detection report: WSL
// availability, version, Hyper-V status, installed distributions, blockers,
// and recommendations. This is exposed via DetectWSL2Status and integrated
// into the EnvironmentInfo struct.
//
// # Zero-Trust Command Execution
//
// The bridge does NOT call exec.Command directly. Instead, it uses an injected
// ExecFunc that routes all commands through the security gate. The default
// (raw exec.Command) is only for standalone/testing use. Production code MUST
// call SetExecFunc(engine.SanitizeAndExecute) at startup.
//
// # Key Types
//
//   - ExecFunc — Function signature for the centralized execution function
//   - EnvironmentInfo — Platform-agnostic environment detection result
//   - WSL2Status — Windows-side WSL2 detection intelligence report
//   - WSLDistro — A single WSL distribution entry
package bridge
