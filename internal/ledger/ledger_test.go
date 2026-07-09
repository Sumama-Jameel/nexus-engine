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
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
)

// newTestState creates a StateTracker that writes to a temp directory.
func newTestState(t *testing.T) *engine.StateTracker {
	t.Helper()
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	state, err := engine.NewStateTracker()
	if err != nil {
		t.Fatalf("NewStateTracker: %v", err)
	}
	return state
}

// fakeInfo returns a deterministic SystemInfo for tests.
func fakeInfo() *engine.SystemInfo {
	return &engine.SystemInfo{
		OS:         "linux",
		Arch:       "amd64",
		Kernel:     "6.8.0-arch1-1",
		Hostname:   "test-machine",
		CPUModel:   "Intel(R) Core(TM) i7-10750H",
		CPUCores:   12,
		RAMTotalMB: 16384,
		DiskTotalGB: 512.0,
		GPU:        "NVIDIA GeForce RTX 3070",
		IsWSL2:     false,
	}
}

// ─── Fingerprint Tests ───

func TestGenerateFingerprint(t *testing.T) {
	info := fakeInfo()
	fp := GenerateFingerprint(info)
	if len(fp) != 32 {
		t.Errorf("fingerprint length = %d, want 32", len(fp))
	}
	for _, c := range fp {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex char %c in fingerprint", c)
		}
	}
}

func TestGenerateFingerprint_Deterministic(t *testing.T) {
	info := fakeInfo()
	fp1 := GenerateFingerprint(info)
	fp2 := GenerateFingerprint(info)
	if fp1 != fp2 {
		t.Error("fingerprint should be deterministic for same input")
	}
}

func TestGenerateFingerprint_DifferentInput(t *testing.T) {
	info1 := fakeInfo()
	info2 := fakeInfo()
	info2.GPU = "AMD Radeon RX 6800"
	fp1 := GenerateFingerprint(info1)
	fp2 := GenerateFingerprint(info2)
	if fp1 == fp2 {
		t.Error("different GPU should produce different fingerprint")
	}
}

func TestGenerateFingerprint_ExcludesPII(t *testing.T) {
	info1 := fakeInfo()
	info2 := fakeInfo()
	info2.Hostname = "different-host"
	fp1 := GenerateFingerprint(info1)
	fp2 := GenerateFingerprint(info2)
	if fp1 != fp2 {
		t.Error("fingerprint should NOT include hostname")
	}
}

func TestGenerateFingerprint_AllFields(t *testing.T) {
	info := &engine.SystemInfo{
		OS:         "darwin",
		Arch:       "arm64",
		Kernel:     "23.0.0",
		Hostname:   "any",
		CPUModel:   "Apple M3 Pro",
		CPUCores:   11,
		RAMTotalMB: 36864,
		DiskTotalGB: 1024.0,
		GPU:        "Apple M3 Pro (19-core)",
		IsWSL2:     false,
	}
	fp := GenerateFingerprint(info)
	if len(fp) != 32 {
		t.Errorf("fingerprint length = %d, want 32", len(fp))
	}
}

func TestShortFingerprint(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"a1b2c3d4e5f6g7h8", "a1b2c3d4"},
		{"short", "short"},
		{"", ""},
		{"12345678", "12345678"},
		{"123456789", "12345678"},
	}
	for _, tc := range tests {
		got := ShortFingerprint(tc.input)
		if got != tc.want {
			t.Errorf("ShortFingerprint(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ─── Record Tests ───

func TestRecord_WithFullData(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()
	info := fakeInfo()

	orchResult := &installer.OrchestratorResult{
		Total:     10,
		Succeeded: 10,
		Failed:    0,
		Aborted: false,
	}

	err := Record(ctx, state, info, orchResult, "base-dev", "apt", nil)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	ledger := state.GetLedger()
	if len(ledger.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(ledger.Records))
	}

	r := ledger.Records[0]
	if !r.Success {
		t.Error("expected success=true")
	}
	if r.ProfileName != "base-dev" {
		t.Errorf("ProfileName = %q, want %q", r.ProfileName, "base-dev")
	}
	if r.PackageManager != "apt" {
		t.Errorf("PackageManager = %q, want %q", r.PackageManager, "apt")
	}
	if r.OS != "linux" {
		t.Errorf("OS = %q, want %q", r.OS, "linux")
	}
	if r.GPU != "NVIDIA GeForce RTX 3070" {
		t.Errorf("GPU = %q, want %q", r.GPU, "NVIDIA GeForce RTX 3070")
	}
	if r.ErrorMessage != "" {
		t.Errorf("expected no error message, got %q", r.ErrorMessage)
	}
}

func TestRecord_WithInstallError(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()
	info := fakeInfo()

	installErr := InstallerError("package install failed: timeout")

	err := Record(ctx, state, info, nil, "test-profile", "pacman", installErr)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	ledger := state.GetLedger()
	if len(ledger.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(ledger.Records))
	}

	r := ledger.Records[0]
	if r.Success {
		t.Error("expected success=false when install failed")
	}
	if r.ErrorMessage != "package install failed: timeout" {
		t.Errorf("ErrorMessage = %q, want %q", r.ErrorMessage, "package install failed: timeout")
	}
	if r.ProfileName != "test-profile" {
		t.Errorf("ProfileName = %q, want %q", r.ProfileName, "test-profile")
	}
}

func TestRecord_WithNilInfo(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	err := Record(ctx, state, nil, nil, "", "", nil)
	if err != nil {
		t.Fatalf("Record with nil info should not error: %v", err)
	}

	ledger := state.GetLedger()
	if len(ledger.Records) != 0 {
		t.Errorf("expected 0 records when info is nil, got %d", len(ledger.Records))
	}
}

func TestRecord_WithNilState(t *testing.T) {
	ctx := context.Background()
	err := Record(ctx, nil, fakeInfo(), nil, "", "", nil)
	if err != nil {
		t.Fatalf("Record with nil state should not error: %v", err)
	}
}

func TestRecordSimple(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	err := RecordSimple(ctx, state)
	if err != nil {
		t.Fatalf("RecordSimple failed: %v", err)
	}

	ledger := state.GetLedger()
	if len(ledger.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(ledger.Records))
	}

	r := ledger.Records[0]
	if r.ProfileName != "manual" {
		t.Errorf("ProfileName = %q, want %q", r.ProfileName, "manual")
	}
	if !r.Success {
		t.Error("expected success=true for manual record")
	}
	if r.OS == "" {
		t.Error("expected OS to be populated by probe")
	}
	if r.DeviceFingerprint == "" {
		t.Error("expected DeviceFingerprint to be populated")
	}
}

func TestRecordSimple_NilState(t *testing.T) {
	ctx := context.Background()
	err := RecordSimple(ctx, nil)
	if err != nil {
		t.Fatalf("RecordSimple with nil state should not error: %v", err)
	}
}

func TestRecord_BoundedRing(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	for i := 0; i < engine.MaxLedgerRecords+10; i++ {
		info := fakeInfo()
		info.CPUCores = i + 1
		err := Record(ctx, state, info, &installer.OrchestratorResult{
			Total: 1, Succeeded: 1, Aborted: false,
		}, "test", "apt", nil)
		if err != nil {
			t.Fatalf("Record %d failed: %v", i, err)
		}
	}

	ledger := state.GetLedger()
	if len(ledger.Records) != engine.MaxLedgerRecords {
		t.Errorf("expected %d records, got %d", engine.MaxLedgerRecords, len(ledger.Records))
	}

	// The first record should be dropped, the last should be present
	if ledger.Records[0].CPUCores != 11 {
		t.Errorf("expected first record CPUCores=11, got %d", ledger.Records[0].CPUCores)
	}
}

// ─── Query Tests ───

func TestQueryField_GPU(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()
	info := fakeInfo()

	_ = Record(ctx, state, info, &installer.OrchestratorResult{Total: 1, Succeeded: 1, Aborted: false}, "test", "apt", nil)

	report, err := QueryField(ctx, state, "gpu", "NVIDIA")
	if err != nil {
		t.Fatalf("QueryField failed: %v", err)
	}
	if report.Matches != 1 {
		t.Errorf("expected 1 match, got %d", report.Matches)
	}
	if report.SuccessRate != 1.0 {
		t.Errorf("expected success rate 1.0, got %f", report.SuccessRate)
	}
}

func TestQueryField_NoMatch(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()
	info := fakeInfo()

	_ = Record(ctx, state, info, &installer.OrchestratorResult{Total: 1, Succeeded: 1, Aborted: false}, "test", "apt", nil)

	report, err := QueryField(ctx, state, "gpu", "AMD")
	if err != nil {
		t.Fatalf("QueryField failed: %v", err)
	}
	if report.Matches != 0 {
		t.Errorf("expected 0 matches, got %d", report.Matches)
	}
	if report.SuccessRate != 0.0 {
		t.Errorf("expected success rate 0.0, got %f", report.SuccessRate)
	}
}

func TestQueryField_OS(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()
	info := fakeInfo()

	_ = Record(ctx, state, info, &installer.OrchestratorResult{Total: 1, Succeeded: 1, Aborted: false}, "test", "apt", nil)

	report, err := QueryField(ctx, state, "os", "linux")
	if err != nil {
		t.Fatalf("QueryField failed: %v", err)
	}
	if report.Matches != 1 {
		t.Errorf("expected 1 match, got %d", report.Matches)
	}
}

func TestQueryField_CPU(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()
	info := fakeInfo()

	_ = Record(ctx, state, info, &installer.OrchestratorResult{Total: 1, Succeeded: 1, Aborted: false}, "test", "apt", nil)

	report, err := QueryField(ctx, state, "cpu", "i7")
	if err != nil {
		t.Fatalf("QueryField failed: %v", err)
	}
	if report.Matches != 1 {
		t.Errorf("expected 1 match, got %d", report.Matches)
	}
}

func TestQueryField_Arch(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()
	info := fakeInfo()

	_ = Record(ctx, state, info, &installer.OrchestratorResult{Total: 1, Succeeded: 1, Aborted: false}, "test", "apt", nil)

	report, err := QueryField(ctx, state, "arch", "amd64")
	if err != nil {
		t.Fatalf("QueryField failed: %v", err)
	}
	if report.Matches != 1 {
		t.Errorf("expected 1 match, got %d", report.Matches)
	}
}

func TestQueryField_Kernel(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()
	info := fakeInfo()

	_ = Record(ctx, state, info, &installer.OrchestratorResult{Total: 1, Succeeded: 1, Aborted: false}, "test", "apt", nil)

	report, err := QueryField(ctx, state, "kernel", "6.8")
	if err != nil {
		t.Fatalf("QueryField failed: %v", err)
	}
	if report.Matches != 1 {
		t.Errorf("expected 1 match, got %d", report.Matches)
	}
}

func TestQueryField_EmptyLedger(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	report, err := QueryField(ctx, state, "gpu", "NVIDIA")
	if err != nil {
		t.Fatalf("QueryField on empty ledger: %v", err)
	}
	if report.TotalRecords != 0 {
		t.Errorf("expected TotalRecords=0, got %d", report.TotalRecords)
	}
	if report.Matches != 0 {
		t.Errorf("expected Matches=0, got %d", report.Matches)
	}
}

func TestQueryField_UnsupportedField(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	_, err := QueryField(ctx, state, "unsupported", "value")
	if err == nil {
		t.Fatal("expected error for unsupported field")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error should mention unsupported, got: %v", err)
	}
}

func TestQueryField_EmptyArgs(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	_, err := QueryField(ctx, state, "", "")
	if err == nil {
		t.Fatal("expected error for empty args")
	}
	_, err = QueryField(ctx, state, "gpu", "")
	if err == nil {
		t.Fatal("expected error for empty value")
	}
}

func TestQueryField_SuccessRate(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		info := fakeInfo()
		info.CPUCores = i + 1
		success := i%2 == 0
		aborted := !success
		_ = Record(ctx, state, info, &installer.OrchestratorResult{
			Total:     1,
			Succeeded: map[bool]int{true: 1, false: 0}[success],
			Failed:    map[bool]int{true: 0, false: 1}[success],
			Aborted:   aborted,
		}, "test", "apt", nil)
	}

	report, err := QueryField(ctx, state, "os", "linux")
	if err != nil {
		t.Fatalf("QueryField failed: %v", err)
	}
	if report.Matches != 10 {
		t.Errorf("expected 10 matches, got %d", report.Matches)
	}
	if report.SuccessRate != 0.5 {
		t.Errorf("expected success rate 0.5, got %f", report.SuccessRate)
	}
}

func TestQueryField_NilState(t *testing.T) {
	ctx := context.Background()
	_, err := QueryField(ctx, nil, "gpu", "test")
	if err == nil {
		t.Fatal("expected error for nil state")
	}
}

// ─── CheckHardware Tests ───

func TestCheckHardware_NoRecords(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	report, err := CheckHardware(ctx, state)
	if err != nil {
		t.Fatalf("CheckHardware failed: %v", err)
	}
	if !report.Unknown {
		t.Error("expected Unknown=true when no records")
	}
	if report.HardwareOK {
		t.Error("expected HardwareOK=false when no records")
	}
}

func TestCheckHardware_NilState(t *testing.T) {
	ctx := context.Background()
	_, err := CheckHardware(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil state")
	}
}

func TestCheckHardware_MatchingRecord(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	_ = RecordSimple(ctx, state)

	report, err := CheckHardware(ctx, state)
	if err != nil {
		t.Fatalf("CheckHardware failed: %v", err)
	}
	if report.Unknown {
		t.Error("expected Unknown=false when matching records exist")
	}
	if !report.HardwareOK {
		t.Errorf("expected HardwareOK=true for self-match, got rate=%.2f records=%d",
			report.SuccessRate, report.TotalRecords)
	}
}

// ─── Sync Tests ───

func TestSync_NilState(t *testing.T) {
	ctx := context.Background()
	err := Sync(ctx, SyncDeps{State: nil})
	if err == nil {
		t.Fatal("expected error for nil state")
	}
}

func TestSync_SyncNotEnabled(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	_ = RecordSimple(ctx, state)

	err := Sync(ctx, SyncDeps{State: state})
	if err == nil {
		t.Fatal("expected error when sync not enabled")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("error should mention 'not enabled', got: %v", err)
	}
}

func TestSync_NoRecords(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	_ = state.SetCommunitySyncEnabled(true)

	err := Sync(ctx, SyncDeps{State: state})
	if err == nil {
		t.Fatal("expected error when no records")
	}
	if !strings.Contains(err.Error(), "no ledger records") {
		t.Errorf("error should mention 'no ledger records', got: %v", err)
	}
}

func TestSync_Success(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	_ = RecordSimple(ctx, state)
	_ = state.SetCommunitySyncEnabled(true)

	err := Sync(ctx, SyncDeps{State: state})
	if err != nil {
		t.Fatalf("Sync should succeed: %v", err)
	}
}

// ─── Pull Tests ───

func TestPull_NilState(t *testing.T) {
	ctx := context.Background()
	err := Pull(ctx, SyncDeps{State: nil})
	if err == nil {
		t.Fatal("expected error for nil state")
	}
}

func TestPull_Success(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	err := Pull(ctx, SyncDeps{State: state})
	if err != nil {
		t.Fatalf("Pull should succeed: %v", err)
	}
}

// ─── State Integration Tests ───

func TestRecordLedgerEntry(t *testing.T) {
	state := newTestState(t)

	report := engine.HardwareReport{
		DeviceFingerprint: "test-fp-1234",
		OS:                "linux",
		Arch:              "amd64",
		CPUModel:          "Test CPU",
		CPUCores:          8,
		RAMTotalMB:        16384,
		GPU:               "Test GPU",
		Success:           true,
		ProfileName:       "test-profile",
		RecordedAt:        time.Now().UTC(),
	}

	err := state.RecordLedgerEntry(report)
	if err != nil {
		t.Fatalf("RecordLedgerEntry failed: %v", err)
	}

	ledger := state.GetLedger()
	if len(ledger.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(ledger.Records))
	}
	if ledger.Records[0].DeviceFingerprint != "test-fp-1234" {
		t.Errorf("fingerprint = %q, want %q", ledger.Records[0].DeviceFingerprint, "test-fp-1234")
	}
}

func TestSetCommunitySyncEnabled(t *testing.T) {
	state := newTestState(t)

	if err := state.SetCommunitySyncEnabled(true); err != nil {
		t.Fatalf("SetCommunitySyncEnabled(true): %v", err)
	}

	ledger := state.GetLedger()
	if !ledger.CommunitySyncEnabled {
		t.Error("expected CommunitySyncEnabled=true")
	}

	if err := state.SetCommunitySyncEnabled(false); err != nil {
		t.Fatalf("SetCommunitySyncEnabled(false): %v", err)
	}

	ledger = state.GetLedger()
	if ledger.CommunitySyncEnabled {
		t.Error("expected CommunitySyncEnabled=false")
	}
}

func TestRecordLedgerSync(t *testing.T) {
	state := newTestState(t)

	if err := state.RecordLedgerSync(); err != nil {
		t.Fatalf("RecordLedgerSync failed: %v", err)
	}

	ledger := state.GetLedger()
	if ledger.LastSyncedAt.IsZero() {
		t.Error("expected LastSyncedAt to be set")
	}
}

func TestGetLedger_Empty(t *testing.T) {
	state := newTestState(t)

	ledger := state.GetLedger()
	if len(ledger.Records) != 0 {
		t.Errorf("expected empty records, got %d", len(ledger.Records))
	}
	if !ledger.LastAnalyzedAt.IsZero() {
		t.Error("expected zero LastAnalyzedAt on empty ledger")
	}
}

func TestGetLedger_ReturnsCopy(t *testing.T) {
	state := newTestState(t)

	_ = state.RecordLedgerEntry(engine.HardwareReport{
		DeviceFingerprint: "fp-1",
		OS:                "linux",
		RecordedAt:        time.Now().UTC(),
	})

	ledger := state.GetLedger()
	ledger.Records[0].OS = "mutated"
	ledger.Records = append(ledger.Records, engine.HardwareReport{})

	newLedger := state.GetLedger()
	if len(newLedger.Records) != 1 {
		t.Error("GetLedger should return a copy — mutation should not affect original")
	}
	if newLedger.Records[0].OS != "linux" {
		t.Error("GetLedger should return a copy — field mutation should not affect original")
	}
}

// ─── Edge Cases ───

func TestRecord_MultipleEntries(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		info := fakeInfo()
		info.CPUCores = i + 1
		_ = Record(ctx, state, info, &installer.OrchestratorResult{
			Total: 1, Succeeded: 1, Aborted: false,
		}, "test", "apt", nil)
	}

	ledger := state.GetLedger()
	if len(ledger.Records) != 5 {
		t.Errorf("expected 5 records, got %d", len(ledger.Records))
	}
}

func TestRecord_AbortedInstall(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()
	info := fakeInfo()

	orchResult := &installer.OrchestratorResult{
		Total:   5,
		Succeeded: 2,
		Failed:  0,
		Aborted: true,
	}

	err := Record(ctx, state, info, orchResult, "test", "apt", nil)
	if err != nil {
		t.Fatalf("Record with aborted install: %v", err)
	}

	ledger := state.GetLedger()
	if ledger.Records[0].Success {
		t.Error("expected Success=false for aborted install")
	}
}

func TestRecord_NilOrchResultNoError(t *testing.T) {
	state := newTestState(t)
	ctx := context.Background()
	info := fakeInfo()

	err := Record(ctx, state, info, nil, "test", "apt", nil)
	if err != nil {
		t.Fatalf("Record with nil orchResult: %v", err)
	}

	ledger := state.GetLedger()
	if ledger.Records[0].Success {
		t.Error("expected Success=false when no orchestrator result and no error")
	}
}

// This helper exists because installer.OrchestratorResult doesn't export a simple
// error type, so we define one for the test.
type InstallerError string

func (e InstallerError) Error() string { return string(e) }
