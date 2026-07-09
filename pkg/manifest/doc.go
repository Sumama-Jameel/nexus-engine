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

// Package manifest provides Nexus Profile parsing, validation, storage, and
// resolution for the Nexus Protocol engine.
//
// # Overview
//
// The manifest package is the PUBLIC API of the Nexus Profile system. Per the
// Nexus Protocol: "We do not write installation scripts; we write a schema."
// Profiles are declarative YAML documents that specify which packages to
// install, organized by OS family. The engine translates these declarations
// into safe, auditable package manager commands.
//
// # Architecture
//
// The package implements a three-layer architecture:
//
//  1. Validation Layer — Two-gate validation:
//     a) JSON Schema validation enforces structural constraints (patterns,
//     additionalProperties: false, minItems, maxLength)
//     b) Go-level semantic validation enforces business rules (allowed
//     families, non-empty packages, no self-extends)
//
//  2. Storage Layer — ProfileStore manages profiles at ~/.nexus/profiles/
//     with a JSON registry tracking provenance, integrity hashes, and
//     timestamps. Bundled defaults are embedded in the binary; user profiles
//     live on disk.
//
//  3. Resolution Layer — ResolveExtends handles profile inheritance with
//     cycle detection and depth limits. ResolveTarget maps profiles to the
//     detected package manager.
//
// # Security Model
//
// The manifest package enforces several security properties:
//
//   - No Arbitrary Code: Profiles can ONLY declare package names. There is
//     no "run:" directive and no way to execute arbitrary scripts. Per the
//     Protocol: "We will never allow a run: arbitrary_script.sh."
//   - Schema Enforcement: Every profile is validated against a compiled-in
//     JSON Schema. This prevents structural attacks (extra fields, pattern
//     violations, missing required fields).
//   - Integrity Verification: On every load, the profile's SHA256 hash is
//     recomputed and compared against the registry. Mismatch = REJECT.
//     Per V3: "On EVERY load, recompute and compare."
//   - SSRF Protection: Remote profile fetching is restricted to whitelisted
//     HTTPS hosts (AllowedRemoteHosts). URLs with userinfo, query params,
//     or fragments are rejected. Response size is limited to 1MB.
//   - Path Traversal Prevention: Profile names are validated against the
//     pattern ^[a-z0-9][a-z0-9-]*$ and checked for path separators and
//     traversal sequences before any filesystem operations.
//   - Extends Chain Safety: Profile inheritance is bounded by MaxExtendsDepth
//     (5) and cycle detection. Every profile in the chain is individually
//     validated before merging.
//
// # Profile Lifecycle
//
// A profile flows through these stages:
//
//  1. Source — Bundled (embedded), Local (user-created), or Remote (fetched)
//  2. Parse — YAML → NexusProfile struct
//  3. Validate — JSON Schema + semantic validation
//  4. Store — Save to disk with SHA256 hash and metadata
//  5. Resolve — Resolve extends chain and target family
//  6. Apply — Pass resolved packages to the Orchestrator (NOT this package)
//
// # Key Types
//
//   - NexusProfile — A validated configuration manifest
//   - TargetConfig — Package list for a specific OS family
//   - ProfileStore — Local profile directory manager
//   - ProfileMeta — Provenance and integrity metadata
//   - ProfileRegistry — Index file for the profile store
//   - ProfileLoader — Interface for loading profiles by name
//   - ProfileSource — Origin indicator (bundled, local, remote)
//
// # Important Constants
//
//   - MaxExtendsDepth — Maximum depth for extends chain resolution (5)
//   - AllowedPackageFamilies — Whitelist of permitted OS families
//   - AllowedRemoteHosts — Whitelist of permitted remote fetch domains
//   - DefaultRemoteURL — Default GitHub URL for community profiles
package manifest
