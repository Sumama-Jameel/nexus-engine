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

// Package wsl provides WSL2 rootfs management, secure downloading, and
// distribution import for the Nexus Protocol engine.
//
// # Architecture
//
// The wsl package implements V5 "The Instant Linux Importer (The Bridge)" —
// the ability to download a minimal Linux rootfs and import it into WSL2
// automatically, achieving the "60-second promise" from a fresh Windows
// install to a running Nexus development environment.
//
// The package is structured around three core components:
//
//   - RootFS Registry — Defines available Linux images (Alpine, Debian) with
//     hardcoded SHA256 hashes for supply-chain integrity
//   - Secure Downloader — Enterprise-grade HTTP client with SSRF protection,
//     SHA256 verification, and atomic file writes
//   - WSL2 Importer — A 7-step orchestrated import flow that mirrors the
//     V2 Orchestrator pattern
//
// # Platform Availability
//
// WSL2 import is a Windows-only operation. On Linux, the package provides
// stub types and functions that return "not available" errors. The
// IsImportAvailable function should be checked before attempting any
// import operations.
//
// # The 7-Step Import Flow
//
// The WSL2Importer.Import method executes the following steps:
//
//  1. PRE-FLIGHT   — Verify WSL2 readiness (architecture, WSL version, name conflicts)
//  2. DOWNLOAD     — Fetch rootfs tarball with SHA256 verification
//  3. CONFIGURE    — Inject wsl.conf and setup script into tarball before first boot
//  4. IMPORT       — Execute `wsl --import` via the Zero-Trust security gate
//  5. VERIFY       — Confirm the distribution appears in `wsl --list`
//  6. CONFIGURE-POST — Execute embedded setup script (create user, set defaults)
//  7. RECORD       — Record the instance in state and audit log
//
// Each step is independently verifiable, auditable, and recoverable.
//
// # Security Model
//
// The package enforces multiple security layers:
//
//   - SSRF Protection: The Downloader rejects private IP ranges (RFC 1918),
//     loopback, and link-local addresses at the DNS resolution level
//   - HTTPS-Only: All download URLs must use HTTPS; HTTP is rejected
//   - SHA256 Integrity: All downloads are verified against hardcoded hashes
//     using constant-time comparison (crypto/subtle.ConstantTimeCompare)
//     to prevent timing attacks
//   - Atomic Writes: Downloads go to a temp file and are renamed only after
//     verification, preventing partial/corrupted files
//   - Clean Room Model: The injected wsl.conf sets noexec on automounts,
//     preventing cross-OS malware execution
//   - Zero-Trust Execution: All WSL commands route through the injected
//     ExecFunc (engine.SanitizeAndExecute), never raw exec.Command
//   - Input Validation: Distribution names and install paths are validated
//     before use in any command
//
// # Key Types
//
//   - RootFSImage — A Linux rootfs tarball specification with SHA256 hash
//   - Downloader — Secure HTTP client for rootfs downloads
//   - WSL2Importer — Orchestrator for the 7-step import flow
//   - ImportConfig — Configuration for an import operation
//   - ImportResult — Outcome of an import operation
//   - NexusDistro — A WSL2 distribution managed by Nexus
package wsl
