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

// Package engineutil provides shared helper functions used across multiple
// internal packages to avoid code duplication. These are simple, stateless
// utilities that don't depend on any specific package's types.
package engineutil

import (
	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// BoolToResult converts a boolean success flag to a human-readable string.
// Used by orchestrator, runner, and mode packages for audit logging and
// result formatting.
func BoolToResult(b bool) string {
	if b {
		return "success"
	}
	return "failure"
}

// LogAudit writes an audit entry when an AuditLogger is provided.
// Failures are best-effort and do not propagate. This is the shared
// implementation used by dotfiles, mode, and installer packages.
func LogAudit(a *engine.AuditLogger, action, result, detail string) {
	if a == nil {
		return
	}
	_ = a.Log(engine.AuditEntry{
		Action:  action,
		Result:  result,
		Package: detail,
	})
}
