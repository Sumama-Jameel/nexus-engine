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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// newAuditLoggerForTest creates an AuditLogger with a temp directory.
// ---------------------------------------------------------------------------

func newAuditLoggerForTest(t *testing.T) *AuditLogger {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open audit log: %v", err)
	}

	return &AuditLogger{path: path, file: file}
}

// ---------------------------------------------------------------------------
// NewAuditLogger — creation and file handling
// ---------------------------------------------------------------------------

func TestNewAuditLogger(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	logger, err := NewAuditLogger()
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}
	defer logger.Close()

	if logger == nil {
		t.Fatal("NewAuditLogger returned nil")
	}
	if logger.file == nil {
		t.Fatal("file should not be nil")
	}
}

func TestNewAuditLogger_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	logger, err := NewAuditLogger()
	if err != nil {
		t.Fatalf("NewAuditLogger failed: %v", err)
	}
	defer logger.Close()

	nexusDir := filepath.Join(tmpDir, ".nexus")
	info, err := os.Stat(nexusDir)
	if err != nil {
		t.Fatalf("expected ~/.nexus directory: %v", err)
	}
	if !info.IsDir() {
		t.Error("~/.nexus should be a directory")
	}
}

func TestNewAuditLogger_OpensExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	nexusDir := filepath.Join(tmpDir, ".nexus")
	os.MkdirAll(nexusDir, 0755)

	// Pre-create the audit log with some content
	existing := filepath.Join(nexusDir, "audit.log")
	os.WriteFile(existing, []byte("{\"test\":true}\n"), 0644)

	logger, err := NewAuditLogger()
	if err != nil {
		t.Fatalf("NewAuditLogger with existing file failed: %v", err)
	}
	defer logger.Close()

	// The existing content should still be there
	data, _ := os.ReadFile(existing)
	if !strings.Contains(string(data), "test") {
		t.Error("existing audit log content should be preserved")
	}
}

// ---------------------------------------------------------------------------
// Log — writes valid JSONL entries
// ---------------------------------------------------------------------------

func TestAuditLogger_Log(t *testing.T) {
	logger := newAuditLoggerForTest(t)
	defer logger.Close()

	entry := AuditEntry{
		Action:         "install",
		Package:        "git",
		Result:         "success",
		PackageManager: "apt",
		Profile:        "base-dev",
		DurationMs:     1500,
		Verified:       true,
	}

	err := logger.Log(entry)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Read the log file and verify content
	data, err := os.ReadFile(logger.path)
	if err != nil {
		t.Fatalf("failed to read audit log: %v", err)
	}

	var logged AuditEntry
	if err := json.Unmarshal(data, &logged); err != nil {
		t.Fatalf("audit log entry is not valid JSON: %v\ncontent: %s", err, string(data))
	}

	if logged.Action != "install" {
		t.Errorf("Action = %q, want %q", logged.Action, "install")
	}
	if logged.Package != "git" {
		t.Errorf("Package = %q, want %q", logged.Package, "git")
	}
	if logged.Result != "success" {
		t.Errorf("Result = %q, want %q", logged.Result, "success")
	}
	if logged.PackageManager != "apt" {
		t.Errorf("PackageManager = %q, want %q", logged.PackageManager, "apt")
	}
	if logged.DurationMs != 1500 {
		t.Errorf("DurationMs = %d, want %d", logged.DurationMs, 1500)
	}
}

func TestAuditLogger_LogSetsTimestamp(t *testing.T) {
	logger := newAuditLoggerForTest(t)
	defer logger.Close()

	entry := AuditEntry{
		Action:  "install",
		Package: "git",
		Result:  "success",
	}

	before := time.Now().UTC()
	_ = logger.Log(entry)
	after := time.Now().UTC()

	data, _ := os.ReadFile(logger.path)
	var logged AuditEntry
	json.Unmarshal(data, &logged)

	// Parse the timestamp
	ts, err := time.Parse(time.RFC3339Nano, logged.Timestamp)
	if err != nil {
		t.Fatalf("failed to parse timestamp %q: %v", logged.Timestamp, err)
	}

	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not between before=%v and after=%v", ts, before, after)
	}
}

// ---------------------------------------------------------------------------
// Log — entries accumulate (append-only)
// ---------------------------------------------------------------------------

func TestAuditLogger_AppendOnly(t *testing.T) {
	logger := newAuditLoggerForTest(t)
	defer logger.Close()

	entries := []AuditEntry{
		{Action: "install", Package: "git", Result: "success"},
		{Action: "install", Package: "curl", Result: "success"},
		{Action: "install", Package: "vim", Result: "failure", Error: "not found"},
		{Action: "verify", Package: "git", Result: "success"},
		{Action: "remove", Package: "vim", Result: "success"},
	}

	for _, entry := range entries {
		if err := logger.Log(entry); err != nil {
			t.Fatalf("Log failed: %v", err)
		}
	}

	// Read directly from the file since ReadAuditLog uses HOME which points elsewhere
	data, err := os.ReadFile(logger.path)
	if err != nil {
		t.Fatalf("failed to read audit log: %v", err)
	}
	lines := splitLines(strings.TrimSpace(string(data)))
	if len(lines) != len(entries) {
		t.Errorf("expected %d log lines, got %d", len(entries), len(lines))
	}

	// Verify order (should be same as insertion order)
	for i, line := range lines {
		var logged AuditEntry
		if err := json.Unmarshal([]byte(line), &logged); err != nil {
			t.Errorf("line %d is not valid JSON: %v", i, err)
			continue
		}
		if logged.Action != entries[i].Action {
			t.Errorf("entry[%d].Action = %q, want %q", i, logged.Action, entries[i].Action)
		}
		if logged.Package != entries[i].Package {
			t.Errorf("entry[%d].Package = %q, want %q", i, logged.Package, entries[i].Package)
		}
	}
}

// ---------------------------------------------------------------------------
// Log — format validation (each line is valid JSON with required fields)
// ---------------------------------------------------------------------------

func TestAuditLogger_EntryFormat(t *testing.T) {
	logger := newAuditLoggerForTest(t)
	defer logger.Close()

	entry := AuditEntry{
		Action:         "install",
		Package:        "git",
		Result:         "success",
		PackageManager: "apt",
		Profile:        "base-dev",
		DurationMs:     500,
		Verified:       true,
		Error:          "",
		Target:         "",
	}

	_ = logger.Log(entry)

	data, _ := os.ReadFile(logger.path)
	line := strings.TrimSpace(string(data))

	// Each line must be valid JSON
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		t.Fatalf("audit line is not valid JSON: %v\nline: %s", err, line)
	}

	// Required fields must be present
	requiredFields := []string{"timestamp", "action", "package", "result"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("audit entry missing required field %q", field)
		}
	}
}

func TestAuditLogger_OmittedFields(t *testing.T) {
	logger := newAuditLoggerForTest(t)
	defer logger.Close()

	// Entry with minimal fields — optional fields should be omitted
	entry := AuditEntry{
		Action:  "verify",
		Package: "curl",
		Result:  "success",
	}

	_ = logger.Log(entry)

	data, _ := os.ReadFile(logger.path)
	line := strings.TrimSpace(string(data))

	var raw map[string]interface{}
	json.Unmarshal([]byte(line), &raw)

	// Optional empty fields should be omitted (omitempty)
	if _, ok := raw["error"]; ok {
		t.Error("empty 'error' field should be omitted with omitempty")
	}
	if _, ok := raw["package_manager"]; ok {
		t.Error("empty 'package_manager' should be omitted with omitempty")
	}
}

func TestAuditLogger_FailureEntryWithError(t *testing.T) {
	logger := newAuditLoggerForTest(t)
	defer logger.Close()

	entry := AuditEntry{
		Action:  "install",
		Package: "nonexistent",
		Result:  "failure",
		Error:   "PACKAGE_NOT_FOUND",
	}

	_ = logger.Log(entry)

	data, _ := os.ReadFile(logger.path)
	var logged AuditEntry
	json.Unmarshal(data, &logged)

	if logged.Result != "failure" {
		t.Errorf("Result = %q, want %q", logged.Result, "failure")
	}
	if logged.Error != "PACKAGE_NOT_FOUND" {
		t.Errorf("Error = %q, want %q", logged.Error, "PACKAGE_NOT_FOUND")
	}
}

// ---------------------------------------------------------------------------
// Close — safe to call multiple times
// ---------------------------------------------------------------------------

func TestAuditLogger_Close(t *testing.T) {
	logger := newAuditLoggerForTest(t)

	err := logger.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Close again should not panic
	err = logger.Close()
	if err != nil {
		t.Logf("second Close returned: %v (acceptable)", err)
	}
}

func TestAuditLogger_LogAfterClose(t *testing.T) {
	logger := newAuditLoggerForTest(t)
	logger.Close()

	entry := AuditEntry{Action: "install", Package: "git", Result: "success"}
	err := logger.Log(entry)
	if err == nil {
		t.Error("Log after Close should return an error")
	}
}

// ---------------------------------------------------------------------------
// ReadAuditLog
// ---------------------------------------------------------------------------

func TestReadAuditLog_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	entries, err := ReadAuditLog(10)
	if err != nil {
		t.Fatalf("ReadAuditLog on non-existent file should not error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// splitLines — unit test
// ---------------------------------------------------------------------------

func TestSplitLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"single_line", "hello", []string{"hello"}},
		{"two_lines", "hello\nworld", []string{"hello", "world"}},
		{"trailing_newline", "hello\n", []string{"hello"}},
		{"empty_line_between", "hello\n\nworld", []string{"hello", "world"}},
		{"multiple_empty_lines", "a\n\n\nb", []string{"a", "b"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := splitLines(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("splitLines(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.expected, len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitLines(%q)[%d] = %q, want %q",
						tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AuditEntry — WSL-specific fields
// ---------------------------------------------------------------------------

func TestAuditEntry_WSLFields(t *testing.T) {
	logger := newAuditLoggerForTest(t)
	defer logger.Close()

	entry := AuditEntry{
		Action:  "wsl_import",
		Package: "ubuntu-22.04",
		Result:  "success",
		Target:  "ubuntu-22.04",
	}

	_ = logger.Log(entry)

	data, _ := os.ReadFile(logger.path)
	var logged AuditEntry
	json.Unmarshal(data, &logged)

	if logged.Target != "ubuntu-22.04" {
		t.Errorf("Target = %q, want %q", logged.Target, "ubuntu-22.04")
	}
	if logged.Action != "wsl_import" {
		t.Errorf("Action = %q, want %q", logged.Action, "wsl_import")
	}
}

// ---------------------------------------------------------------------------
// ReadAuditLog — edge case tests
// ---------------------------------------------------------------------------

func TestReadAuditLog_WithEntries(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create a valid JSONL audit log
	nexusDir := filepath.Join(tmpDir, ".nexus")
	os.MkdirAll(nexusDir, 0755)

	content := `{"timestamp":"2024-01-01T00:00:00Z","action":"install","package":"git","result":"success"}
{"timestamp":"2024-01-01T00:01:00Z","action":"install","package":"curl","result":"success"}
{"timestamp":"2024-01-01T00:02:00Z","action":"remove","package":"vim","result":"failure","error":"NOT_MANAGED"}
`
	logPath := filepath.Join(nexusDir, "audit.log")
	os.WriteFile(logPath, []byte(content), 0644)

	entries, err := ReadAuditLog(0)
	if err != nil {
		t.Fatalf("ReadAuditLog failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if entries[0].Action != "install" || entries[0].Package != "git" {
		t.Errorf("entry[0] = {Action: %q, Package: %q}, want install/git", entries[0].Action, entries[0].Package)
	}
	if entries[1].Package != "curl" {
		t.Errorf("entry[1].Package = %q, want %q", entries[1].Package, "curl")
	}
	if entries[2].Action != "remove" || entries[2].Result != "failure" {
		t.Errorf("entry[2] = {Action: %q, Result: %q}, want remove/failure", entries[2].Action, entries[2].Result)
	}
}

func TestReadAuditLog_WithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	nexusDir := filepath.Join(tmpDir, ".nexus")
	os.MkdirAll(nexusDir, 0755)

	content := `{"timestamp":"2024-01-01T00:00:00Z","action":"install","package":"git","result":"success"}
{"timestamp":"2024-01-01T00:01:00Z","action":"install","package":"curl","result":"success"}
{"timestamp":"2024-01-01T00:02:00Z","action":"install","package":"vim","result":"success"}
{"timestamp":"2024-01-01T00:03:00Z","action":"install","package":"htop","result":"success"}
{"timestamp":"2024-01-01T00:04:00Z","action":"install","package":"tmux","result":"success"}
`
	logPath := filepath.Join(nexusDir, "audit.log")
	os.WriteFile(logPath, []byte(content), 0644)

	// Request only the last 2 entries
	entries, err := ReadAuditLog(2)
	if err != nil {
		t.Fatalf("ReadAuditLog failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Package != "htop" {
		t.Errorf("first entry package = %q, want %q", entries[0].Package, "htop")
	}
	if entries[1].Package != "tmux" {
		t.Errorf("second entry package = %q, want %q", entries[1].Package, "tmux")
	}
}

func TestReadAuditLog_LimitLargerThanEntries(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	nexusDir := filepath.Join(tmpDir, ".nexus")
	os.MkdirAll(nexusDir, 0755)

	content := `{"timestamp":"2024-01-01T00:00:00Z","action":"install","package":"git","result":"success"}
`
	logPath := filepath.Join(nexusDir, "audit.log")
	os.WriteFile(logPath, []byte(content), 0644)

	// Request 100 entries when there's only 1
	entries, err := ReadAuditLog(100)
	if err != nil {
		t.Fatalf("ReadAuditLog failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestReadAuditLog_InvalidJSONSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	nexusDir := filepath.Join(tmpDir, ".nexus")
	os.MkdirAll(nexusDir, 0755)

	content := `{"timestamp":"2024-01-01T00:00:00Z","action":"install","package":"git","result":"success"}
INVALID LINE HERE
{"timestamp":"2024-01-01T00:02:00Z","action":"remove","package":"vim","result":"success"}
`
	logPath := filepath.Join(nexusDir, "audit.log")
	os.WriteFile(logPath, []byte(content), 0644)

	entries, err := ReadAuditLog(0)
	if err != nil {
		t.Fatalf("ReadAuditLog failed: %v", err)
	}
	// Invalid lines should be silently skipped
	if len(entries) != 2 {
		t.Fatalf("expected 2 valid entries (invalid skipped), got %d", len(entries))
	}
	if entries[0].Package != "git" {
		t.Errorf("entry[0].Package = %q, want %q", entries[0].Package, "git")
	}
	if entries[1].Package != "vim" {
		t.Errorf("entry[1].Package = %q, want %q", entries[1].Package, "vim")
	}
}

func TestReadAuditLog_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	nexusDir := filepath.Join(tmpDir, ".nexus")
	os.MkdirAll(nexusDir, 0755)

	logPath := filepath.Join(nexusDir, "audit.log")
	os.WriteFile(logPath, []byte(""), 0644)

	entries, err := ReadAuditLog(0)
	if err != nil {
		t.Fatalf("ReadAuditLog on empty file should not error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries from empty file, got %d", len(entries))
	}
}
