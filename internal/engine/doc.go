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

// Package engine implements the "Brain" of Nexus Protocol — the centralized
// execution and orchestration engine that drives all system-level operations.
//
// # Architecture Overview
//
// The engine operates as a Zero-Trust execution layer between the Nexus CLI
// and the underlying operating system. Every system call passes through a
// security gate (SanitizeAndExecute) that enforces command whitelisting,
// argument sanitization, and mandatory timeouts. No command is ever executed
// through a shell interpreter, eliminating an entire class of injection attacks.
//
// The engine is organized into six core subsystems:
//
//   - Execution (execute.go): The security gate. SanitizeAndExecute validates,
//     sanitizes, and executes all system commands through a strict whitelist and
//     metacharacter rejection pipeline. This is the most security-critical module
//     in the entire codebase.
//   - Probing (probe.go): Discovers the runtime environment by concurrently
//     querying OS, hardware, network, and virtualization state. The resulting
//     SystemInfo drives all downstream decisions about package managers, profiles,
//     and platform-specific behavior.
//   - Configuration (config.go, configure.go): Manages persistent configuration
//     through Viper with a layered search path (cwd → ~/.nexus → /etc/nexus), and
//     performs the CONFIGURE step of nexus init — creating directories, initializing
//     Chezmoi, and injecting Nexus-optimized shell configurations.
//   - State (state.go): Tracks all Nexus-managed packages and WSL2 instances in
//     an atomically-written JSON state file at ~/.nexus/state.json. The state
//     tracker enables idempotent operations, drift detection, and safe rollback.
//   - Audit (audit.go): Provides an append-only, tamper-evident audit trail at
//     ~/.nexus/audit.log in JSONL format. Every install, remove, update, and verify
//     operation is logged with timestamps and durations for full traceability.
//
// # Key Concepts
//
// Zero-Trust Execution: The engine never trusts input from any source. All
// commands are validated against AllowedCommands, all arguments are scanned for
// shell metacharacters, and all operations are bounded by timeouts. The
// containsShellMetacharacters function rejects characters like ;, |, &, $, `,
// and others that could enable command injection.
//
// Idempotent Operations: Every action the engine performs is designed to be
// safely re-executable. Installing an already-installed package is a no-op.
// Re-applying a profile updates environment variables without duplication.
// The state tracker ensures the engine always knows what it has already done.
//
// Platform Awareness: The engine adapts its behavior based on the probed
// environment. On Linux it uses `free -m` for RAM, on Windows it falls back to
// systeminfo. Package manager detection (apt, dnf, pacman, apk) is driven by
// what is actually available on the host. WSL2 detection enables the Bridge
// subsystem for cross-platform Linux environments.
//
// Atomic State Persistence: State is written to a temporary file and renamed
// into place, leveraging POSIX atomic rename semantics to prevent corruption
// from interrupted writes. The state file is mutex-protected for concurrent
// access safety.
//
// # Threat Model
//
// The engine's primary security boundary is SanitizeAndExecute. It defends
// against the following threat vectors:
//
//   - Command Injection: Arguments are scanned for shell metacharacters before
//     execution. Commands are executed directly via exec.CommandContext, never
//     through /bin/sh, so shell expansion is impossible.
//   - Unauthorized Command Execution: Only commands in the AllowedCommands
//     whitelist can be executed. Any attempt to run an unlisted command returns
//     an error before reaching the operating system.
//   - Denial of Service via Hung Processes: All commands are bounded by a
//     60-second timeout (CommandTimeoutSec). If a process does not complete
//     within this window, it is killed via context cancellation.
//   - Environment Variable Injection: Profile environment variables are
//     validated through the same metacharacter scanner before being written
//     to shell configuration files.
package engine
