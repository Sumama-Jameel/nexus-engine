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

package ledger

import (
	"crypto/sha256"
	"fmt"
	"strconv"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// GenerateFingerprint creates a deterministic hardware identifier from a
// SystemInfo snapshot. The fingerprint is used for dedup and matching
// against community records.
//
// Inputs (non-PII only):
//   - OS + Arch + CPUModel + CPUCores + GPU model + RAM (integer MB)
//
// NOT included (privacy):
//   - hostname, network IP, disk used/free, uptime, kernel version
//
// The output is the first 16 bytes (32 hex chars) of the SHA256 digest,
// giving 2^128 collision space — astronomically safe for hardware dedup.
func GenerateFingerprint(info *engine.SystemInfo) string {
	h := sha256.New()
	h.Write([]byte(info.OS))
	h.Write([]byte(info.Arch))
	h.Write([]byte(info.CPUModel))
	h.Write([]byte(strconv.Itoa(info.CPUCores)))
	h.Write([]byte(info.GPU))
	h.Write([]byte(strconv.Itoa(info.RAMTotalMB)))
	return fmt.Sprintf("%x", h.Sum(nil)[:16])
}

// ShortFingerprint returns the first 8 chars of a fingerprint for display.
func ShortFingerprint(fp string) string {
	if len(fp) <= 8 {
		return fp
	}
	return fp[:8]
}
