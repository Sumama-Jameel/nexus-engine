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

package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AuditEntry represents a single audit log entry in the Nexus audit trail.
// Each entry is serialized as a JSON line (JSONL format) for efficient parsing
// and is append-only — entries are never modified after being written.
type AuditEntry struct {
	// Timestamp is the UTC timestamp of the audit event, formatted as
	// RFC3339Nano for sub-millisecond precision.
	Timestamp string `json:"timestamp"`
	// Action is the category of operation performed. Valid values include
	// "install", "remove", "update", "verify", "wsl_import", "wsl_remove",
	// and "wsl_setup".
	Action string `json:"action"`
	// Package is the name of the package targeted by the action.
	Package string `json:"package"`
	// Result is the outcome of the action: "success", "failure", or "skipped".
	Result string `json:"result"`
	// PackageManager is the package manager used for the action (e.g.,
	// "apt-get", "dnf"), omitted for non-package operations.
	PackageManager string `json:"package_manager,omitempty"`
	// Profile is the Nexus profile that triggered the action, omitted for
	// operations not associated with a profile.
	Profile string `json:"profile,omitempty"`
	// Error contains the error message if the action failed, omitted on success.
	Error string `json:"error,omitempty"`
	// Target is the WSL distribution name or other operation-specific target,
	// used for WSL import/remove and similar targeted operations.
	Target string `json:"target,omitempty"`
	// DurationMs is the wall-clock duration of the action in milliseconds,
	// used for performance tracking and slow-operation detection.
	DurationMs int64 `json:"duration_ms,omitempty"`
	// Verified indicates whether a post-install verification check confirmed
	// the package is functional, omitted for actions that do not verify.
	Verified bool `json:"verified,omitempty"`
}

// AuditLogger provides an append-only, tamper-evident audit trail.
// Per the Zero-Trust model: every action must be traceable.
// The file is opened with O_APPEND — existing lines are never modified.
type AuditLogger struct {
	file *os.File
	path string
}

// NewAuditLogger creates or opens the audit log at ~/.nexus/audit.log.
func NewAuditLogger() (*AuditLogger, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine home directory: %w", err)
	}

	nexusDir := filepath.Join(homeDir, ".nexus")
	_ = os.MkdirAll(nexusDir, 0755) //nolint:gosec

	path := filepath.Join(nexusDir, "audit.log")

	// Open with O_APPEND and O_CREATE — append-only, create if missing
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}

	return &AuditLogger{path: path, file: file}, nil
}

// Log writes a structured audit entry to the log file.
// Each entry is a single JSON line (JSONL format) for easy parsing.
func (a *AuditLogger) Log(entry AuditEntry) error {
	if a.file == nil {
		return fmt.Errorf("audit log not initialized")
	}

	entry.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	data = append(data, '\n')

	if _, err := a.file.Write(data); err != nil {
		return fmt.Errorf("failed to write audit entry: %w", err)
	}

	// Sync to ensure durability
	return a.file.Sync()
}

// Close closes the audit log file.
func (a *AuditLogger) Close() error {
	if a.file != nil {
		return a.file.Close()
	}
	return nil
}

// ReadAuditLog reads the last N entries from the audit log.
func ReadAuditLog(n int) ([]AuditEntry, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(homeDir, ".nexus", "audit.log")
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return []AuditEntry{}, nil
		}
		return nil, err
	}

	lines := splitLines(string(data))
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	var entries []AuditEntry
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
