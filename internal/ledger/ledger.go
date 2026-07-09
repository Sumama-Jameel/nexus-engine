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

// Package ledger implements V13 "The Hardware Ledger (The Intelligence)".
//
// The ledger records hardware-configuration snapshots paired with install
// outcomes, building a local "what works on this class of hardware" memory.
// Records are stored as a bounded ring inside NexusState and can be
// optionally synced to a community GitHub registry.
//
// Per ADR 012: the ledger is privacy-first. DeviceFingerprint is a SHA256
// of non-PII hardware attributes only. Community sync is opt-in.
package ledger

import (
	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// SyncDeps holds the injected dependencies for future ledger sync operations.
// Sync will use engine.NewSSRFSafeTransport for SSRF-safe HTTP access.
type SyncDeps struct {
	State *engine.StateTracker
}
