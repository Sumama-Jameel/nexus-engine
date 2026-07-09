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

// Package installer is the "Hands" of the Nexus Protocol.
//
// If the Nexus engine is the brain that decides what a system should look like,
// the installer is the pair of hands that makes it so. This package is
// responsible for every interaction with the operating system's package manager
// — installing, removing, updating, verifying, and auditing every package
// operation.
//
// # Architecture
//
// The installer follows a layered architecture rooted in Domain-Driven Design
// and Zero-Trust security principles:
//
//   - PackageManager interface — the bounded-context contract. Every concrete
//     package manager (apt, pacman, dnf, apk) implements this interface. The
//     Orchestrator depends on the abstraction, never on a concrete type.
//
//   - ExecFunc — the Zero-Trust security gate. No installer calls exec.Command
//     directly. Every shell command passes through the injected ExecFunc
//     (typically SanitizeAndExecute), which validates, logs, and rate-limits
//     every invocation. This prevents command injection and ensures full
//     auditability.
//
//   - Orchestrator — the decision engine that governs installation order,
//     failure handling, rollback, and state recording. It enforces the Three
//     Inviolable Rules (see below).
//
//   - PreFlightCheck — the gatekeeper that validates disk space, network
//     connectivity, sudo access, and package-manager lock state before any
//     write operation begins.
//
//   - VerifyInstallation — the post-install auditor. An exit code 0 from a
//     package manager does not guarantee the package works. This step runs
//     binary-level verification for known packages.
//
// # The Three Inviolable Rules
//
// These rules govern every orchestrated installation and may never be violated:
//
//  1. Never retry a failed package. The user decides whether to retry.
//  2. Never skip a failed foundation package — ABORT the entire run. If
//     ca-certificates is broken, nothing that depends on TLS can be trusted.
//  3. Never record an unverified package as installed. An entry in the state
//     tracker means the package was installed AND verified.
//
// # The Rollback Rule
//
// Derived from Rule 2 and the Zero-Trust principle: if the Orchestrator aborts
// due to a foundation failure, it MUST attempt to remove every package it
// successfully installed during that run. Never leave the system in a
// partially-configured, inconsistent state.
//
// # Priority-Based Installation Ordering
//
// Packages are classified into three priority groups that determine installation
// order:
//
//   - PriorityFoundation (1): ca-certificates, gnupg, build-essential — the
//     base layer required by everything else.
//   - PriorityLanguage (2): python3, openjdk, nodejs — language runtimes that
//     depend on the foundation layer.
//   - PriorityTool (3): git, curl, vim — end-user tools that depend on both
//     layers above.
//
// Within each priority group, packages are installed concurrently using
// goroutines for high-speed parallel execution.
//
// # The Seven-Step Orchestrated Flow
//
// The Orchestrator.Install method executes the following pipeline:
//
//  1. PreFlight — validate environment (disk, network, sudo, lock).
//  2. RefreshIndex — update the package manager's local cache.
//  3. Order — sort packages by priority (foundation → language → tool).
//  4. Execute — install each priority group, concurrently within each group.
//  5. Verify — confirm each installed package is actually functional.
//  6. Record — persist verified installations to the state tracker.
//  7. Audit — log every operation to the audit trail.
//
// On foundation failure at step 4: Rollback → Report.
//
// # Supported Package Managers
//
// The package ships with four PackageManager implementations:
//
//   - AptInstaller — Debian and Ubuntu (apt-get, dpkg)
//   - PacmanInstaller — Arch Linux (pacman)
//   - DnfInstaller — Fedora and RHEL (dnf, rpm)
//   - ApkInstaller — Alpine Linux (apk, runs as root)
//
// Adding a new package manager requires only a new struct implementing
// PackageManager and a new case in NewInstaller — zero modifications to
// existing code (Open/Closed Principle).
//
// # Threat Model
//
// Every command execution flows through ExecFunc, which applies:
//   - Command whitelisting — only known, safe commands are permitted.
//   - Argument sanitization — shell metacharacters are rejected.
//   - Rate limiting — rapid-fire command execution is throttled.
//   - Full audit logging — every command, its arguments, and its outcome
//     are recorded for forensic analysis.
//
// This ensures that even a compromised profile cannot escalate privileges
// or inject arbitrary commands through the installer.
package installer
