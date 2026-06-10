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

package installer

import (
        "context"
        "errors"
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "sync"
        "testing"
        "time"

        "github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// ---------------------------------------------------------------------------
// mockPackageManager — implements PackageManager for orchestrator tests
//
// Important: the Orchestrator's installGroup method discards PackageResults
// from pm.Install() and only inspects the returned error. Therefore, this
// mock returns a non-nil error from Install when a package is in installErr,
// matching how installGroup detects failures.
// ---------------------------------------------------------------------------

type mockPackageManager struct {
        mu         sync.Mutex
        name       string
        installed  map[string]bool  // packages reported as installed by IsInstalled
        installErr map[string]error // packages that fail on Install (causes Install to return error)
        removeErr  map[string]error // packages that fail on Remove
        refreshErr error            // error returned by RefreshIndex
}

func newMockPackageManager(name string) *mockPackageManager {
        return &mockPackageManager{
                name:       name,
                installed:  make(map[string]bool),
                installErr: make(map[string]error),
                removeErr:  make(map[string]error),
        }
}

func (m *mockPackageManager) RefreshIndex(ctx context.Context) error {
        return m.refreshErr
}

func (m *mockPackageManager) Install(ctx context.Context, packages []string) ([]PackageResult, error) {
        var results []PackageResult
        for _, pkg := range packages {
                start := time.Now()
                result := PackageResult{Package: pkg, Action: "install"}
                if err, ok := m.installErr[pkg]; ok {
                        result.Success = false
                        result.Error = err.Error()
                        result.Duration = time.Since(start)
                        results = append(results, result)
                        // Return a non-nil error so that installGroup detects the failure.
                        // installGroup discards PackageResult and only checks the error.
                        return results, err
                }
                result.Success = true
                result.Duration = time.Since(start)
                results = append(results, result)
                m.mu.Lock()
                m.installed[pkg] = true
                m.mu.Unlock()
        }
        return results, nil
}

func (m *mockPackageManager) Remove(ctx context.Context, packages []string) ([]PackageResult, error) {
        var results []PackageResult
        for _, pkg := range packages {
                start := time.Now()
                result := PackageResult{Package: pkg, Action: "remove"}
                if err, ok := m.removeErr[pkg]; ok {
                        result.Success = false
                        result.Error = err.Error()
                } else {
                        result.Success = true
                        m.mu.Lock()
                        delete(m.installed, pkg)
                        m.mu.Unlock()
                }
                result.Duration = time.Since(start)
                results = append(results, result)
        }
        return results, nil
}

func (m *mockPackageManager) Update(ctx context.Context, packages []string) ([]PackageResult, error) {
        var results []PackageResult
        for _, pkg := range packages {
                results = append(results, PackageResult{Package: pkg, Action: "update", Success: true})
        }
        return results, nil
}

func (m *mockPackageManager) IsInstalled(ctx context.Context, pkg string) bool {
        m.mu.Lock()
        defer m.mu.Unlock()
        return m.installed[pkg]
}

func (m *mockPackageManager) ListInstalled(ctx context.Context) ([]string, error) {
        m.mu.Lock()
        defer m.mu.Unlock()
        var pkgs []string
        for p := range m.installed {
                pkgs = append(pkgs, p)
        }
        return pkgs, nil
}

func (m *mockPackageManager) Search(ctx context.Context, query string) ([]string, error) {
        return nil, nil
}

func (m *mockPackageManager) Name() string {
        return m.name
}

// ---------------------------------------------------------------------------
// helpers for creating StateTracker and AuditLogger with temp dirs
// ---------------------------------------------------------------------------

// newTestStateTracker creates a StateTracker using a temp directory.
// Since StateTracker's fields are unexported, we redirect HOME so it
// writes to a temp dir instead of the real ~/.nexus.
func newTestStateTracker(t *testing.T) *engine.StateTracker {
        t.Helper()
        tmpDir := t.TempDir()
        origHome := os.Getenv("HOME")
        os.Setenv("HOME", tmpDir)
        t.Cleanup(func() { os.Setenv("HOME", origHome) })

        st, err := engine.NewStateTracker()
        if err != nil {
                t.Fatalf("failed to create StateTracker: %v", err)
        }
        return st
}

// newTestAuditLogger creates an AuditLogger using a temp directory.
func newTestAuditLogger(t *testing.T) *engine.AuditLogger {
        t.Helper()
        tmpDir := t.TempDir()
        origHome := os.Getenv("HOME")
        os.Setenv("HOME", tmpDir)
        t.Cleanup(func() { os.Setenv("HOME", origHome) })

        al, err := engine.NewAuditLogger()
        if err != nil {
                t.Fatalf("failed to create AuditLogger: %v", err)
        }
        t.Cleanup(func() { al.Close() })
        return al
}

// ---------------------------------------------------------------------------
// Orchestrator tests
// ---------------------------------------------------------------------------

func TestOrchestrator_NewOrchestrator(t *testing.T) {
        t.Parallel()

        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()
        o := NewOrchestrator(pm, execFn, nil, nil, "test-profile", false)

        if o == nil {
                t.Fatal("NewOrchestrator returned nil")
        }
        if o.dryRun != false {
                t.Error("dryRun should be false")
        }
        if o.profile != "test-profile" {
                t.Errorf("profile = %q, want %q", o.profile, "test-profile")
        }
        if o.pm == nil {
                t.Error("pm should not be nil")
        }
}

func TestOrchestrator_NewOrchestrator_DryRun(t *testing.T) {
        t.Parallel()

        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()
        o := NewOrchestrator(pm, execFn, nil, nil, "test", true)

        if !o.dryRun {
                t.Error("dryRun should be true")
        }
}

func TestOrchestrator_Install_EmptyPackages(t *testing.T) {
        t.Parallel()

        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()
        o := NewOrchestrator(pm, execFn, nil, nil, "test", false)

        result, err := o.Install(context.Background(), []string{})
        if err != nil {
                t.Fatalf("Install with empty packages should not error: %v", err)
        }
        if result.Total != 0 {
                t.Errorf("Total = %d, want 0", result.Total)
        }
        if result.Aborted {
                t.Error("Aborted should be false for empty input")
        }
        if result.Succeeded != 0 {
                t.Errorf("Succeeded = %d, want 0", result.Succeeded)
        }
}

func TestOrchestrator_Install_DryRun(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()
        o := NewOrchestrator(pm, execFn, nil, nil, "test", true)

        result, err := o.Install(context.Background(), []string{"git", "curl"})
        if err != nil {
                t.Fatalf("DryRun Install should not error: %v", err)
        }

        if result.Total != 2 {
                t.Errorf("Total = %d, want 2", result.Total)
        }
        // In dry run, packages are counted as succeeded but not actually installed
        if result.Succeeded != 2 {
                t.Errorf("Succeeded = %d, want 2", result.Succeeded)
        }
        if pm.IsInstalled(context.Background(), "git") {
                t.Error("git should NOT be marked installed during dry run")
        }
}

func TestOrchestrator_Install_Success(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        state := newTestStateTracker(t)
        audit := newTestAuditLogger(t)

        o := NewOrchestrator(pm, execFn, state, audit, "dev", false)

        result, err := o.Install(context.Background(), []string{"git", "curl"})
        if err != nil {
                t.Fatalf("Install should succeed: %v", err)
        }

        if result.Total != 2 {
                t.Errorf("Total = %d, want 2", result.Total)
        }
        if result.Succeeded != 2 {
                t.Errorf("Succeeded = %d, want 2", result.Succeeded)
        }
        if result.Failed != 0 {
                t.Errorf("Failed = %d, want 0", result.Failed)
        }
        if result.Aborted {
                t.Error("should not be aborted")
        }
        // Both git and curl have verifiable binaries that simpleMockExec will satisfy
        if result.Verified != 2 {
                t.Errorf("Verified = %d, want 2", result.Verified)
        }
}

func TestOrchestrator_Install_FoundationFailure_TriggersRollback(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        // ca-certificates is a foundation package; make it fail
        pm.installErr["ca-certificates"] = fmt.Errorf("foundation package install failed")

        state := newTestStateTracker(t)
        audit := newTestAuditLogger(t)

        o := NewOrchestrator(pm, execFn, state, audit, "dev", false)

        result, err := o.Install(context.Background(), []string{"ca-certificates"})
        if err == nil {
                t.Fatal("Install should return error on foundation failure")
        }
        // Verify the error is a typed NexusError (FOUNDATION_FAIL)
        var nerr *NexusError
        if !errors.As(err, &nerr) {
                t.Fatalf("error should be a *NexusError, got %T: %v", err, err)
        }
        if nerr.Code != "FOUNDATION_FAIL" {
                t.Errorf("NexusError.Code = %q, want %q", nerr.Code, "FOUNDATION_FAIL")
        }
        if nerr.Stage != "Install" {
                t.Errorf("NexusError.Stage = %q, want %q", nerr.Stage, "Install")
        }

        if !result.Aborted {
                t.Error("result should be aborted")
        }
        if !strings.Contains(result.AbortReason, "Foundation") {
                t.Errorf("AbortReason should mention Foundation, got: %q", result.AbortReason)
        }
        if result.Rollback == nil {
                t.Error("Rollback should not be nil on foundation failure")
        }
}

func TestOrchestrator_Install_FoundationFailure_RollbackRemovesInstalled(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        // Make foundation fail
        pm.installErr["ca-certificates"] = fmt.Errorf("foundation failed")

        state := newTestStateTracker(t)
        audit := newTestAuditLogger(t)

        o := NewOrchestrator(pm, execFn, state, audit, "dev", false)

        // Foundation packages (priority 1) are processed before tools (priority 3)
        // ca-certificates will fail, triggering abort before git is even attempted
        result, err := o.Install(context.Background(), []string{"git", "ca-certificates"})
        if err == nil {
                t.Fatal("expected error on foundation failure")
        }

        if !result.Aborted {
                t.Error("result should be aborted")
        }
        if result.Rollback == nil {
                t.Error("Rollback should not be nil")
        }
}

func TestOrchestrator_Install_ToolFailure_NoAbort(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        // Make a tool package fail — should NOT trigger abort/rollback
        pm.installErr["git"] = fmt.Errorf("tool install failed")

        state := newTestStateTracker(t)
        audit := newTestAuditLogger(t)

        o := NewOrchestrator(pm, execFn, state, audit, "dev", false)

        // git is a tool package (priority 3) — failure should not cause abort
        result, err := o.Install(context.Background(), []string{"git"})
        if err != nil {
                t.Fatalf("Tool failure should not cause Install to return error: %v", err)
        }

        if result.Aborted {
                t.Error("Tool failure should NOT trigger abort")
        }
        if result.Failed != 1 {
                t.Errorf("Failed = %d, want 1", result.Failed)
        }
        if result.Rollback != nil {
                t.Error("Tool failure should NOT trigger rollback")
        }
}

func TestOrchestrator_Install_AllAlreadyInstalled(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        // Mark all packages as already installed
        pm.installed["git"] = true
        pm.installed["curl"] = true

        o := NewOrchestrator(pm, execFn, nil, nil, "test", false)

        result, err := o.Install(context.Background(), []string{"git", "curl"})
        if err != nil {
                t.Fatalf("Install should succeed: %v", err)
        }

        if result.Total != 2 {
                t.Errorf("Total = %d, want 2", result.Total)
        }
        if result.Skipped != 2 {
                t.Errorf("Skipped = %d, want 2", result.Skipped)
        }
        if result.Succeeded != 0 {
                t.Errorf("Succeeded = %d, want 0", result.Succeeded)
        }
        if result.Aborted {
                t.Error("should not be aborted")
        }
}

func TestOrchestrator_groupByPriority(t *testing.T) {
        t.Parallel()

        tests := []struct {
                name       string
                packages   []string
                wantGroups map[int][]string
        }{
                {
                        name:     "mixed_priorities",
                        packages: []string{"ca-certificates", "python3", "git", "gnupg", "nodejs", "vim"},
                        wantGroups: map[int][]string{
                                PriorityFoundation: {"ca-certificates", "gnupg"},
                                PriorityLanguage:   {"python3", "nodejs"},
                                PriorityTool:       {"git", "vim"},
                        },
                },
                {
                        name:     "foundation_only",
                        packages: []string{"ca-certificates", "gnupg"},
                        wantGroups: map[int][]string{
                                PriorityFoundation: {"ca-certificates", "gnupg"},
                        },
                },
                {
                        name:     "tools_only",
                        packages: []string{"git", "curl", "vim"},
                        wantGroups: map[int][]string{
                                PriorityTool: {"git", "curl", "vim"},
                        },
                },
                {
                        name:     "languages_only",
                        packages: []string{"python3", "nodejs"},
                        wantGroups: map[int][]string{
                                PriorityLanguage: {"python3", "nodejs"},
                        },
                },
        }

        for _, tt := range tests {
                tt := tt
                t.Run(tt.name, func(t *testing.T) {
                        t.Parallel()
                        got := groupByPriority(tt.packages)

                        for priority, wantPkgs := range tt.wantGroups {
                                gotPkgs, ok := got[priority]
                                if !ok {
                                        t.Errorf("missing priority group %d", priority)
                                        continue
                                }
                                if len(gotPkgs) != len(wantPkgs) {
                                        t.Errorf("priority %d: got %d packages, want %d", priority, len(gotPkgs), len(wantPkgs))
                                        continue
                                }
                                gotSet := make(map[string]bool)
                                for _, p := range gotPkgs {
                                        gotSet[p] = true
                                }
                                for _, p := range wantPkgs {
                                        if !gotSet[p] {
                                                t.Errorf("priority %d: missing package %q", priority, p)
                                        }
                                }
                        }

                        if len(got) != len(tt.wantGroups) {
                                t.Errorf("got %d groups, want %d groups", len(got), len(tt.wantGroups))
                        }
                })
        }
}

func TestOrchestrator_SortedPriorityKeys(t *testing.T) {
        t.Parallel()

        tests := []struct {
                name   string
                groups map[int][]string
                want   []int
        }{
                {
                        name: "all_three_groups",
                        groups: map[int][]string{
                                PriorityTool:       {"git"},
                                PriorityFoundation: {"ca-certificates"},
                                PriorityLanguage:   {"python3"},
                        },
                        want: []int{1, 2, 3},
                },
                {
                        name: "two_groups",
                        groups: map[int][]string{
                                PriorityTool:       {"git"},
                                PriorityFoundation: {"ca-certificates"},
                        },
                        want: []int{1, 3},
                },
                {
                        name:   "empty",
                        groups: map[int][]string{},
                        want:   []int{},
                },
                {
                        name: "single_group",
                        groups: map[int][]string{
                                PriorityLanguage: {"python3"},
                        },
                        want: []int{2},
                },
        }

        for _, tt := range tests {
                tt := tt
                t.Run(tt.name, func(t *testing.T) {
                        t.Parallel()
                        got := SortedPriorityKeys(tt.groups)
                        if len(got) != len(tt.want) {
                                t.Fatalf("got %d keys, want %d keys", len(got), len(tt.want))
                        }
                        for i, v := range got {
                                if v != tt.want[i] {
                                        t.Errorf("key[%d] = %d, want %d", i, v, tt.want[i])
                                }
                        }
                })
        }
}

func TestOrchestrator_FormatOrchestratorResult(t *testing.T) {
        t.Parallel()

        tests := []struct {
                name         string
                result       *OrchestratorResult
                wantContains []string
        }{
                {
                        name: "success",
                        result: &OrchestratorResult{
                                Total:     3,
                                Succeeded: 2,
                                Skipped:   1,
                                Verified:  2,
                                Duration:  100 * time.Millisecond,
                        },
                        wantContains: []string{"Total: 3", "Succeeded: 2", "Skipped: 1", "Verified: 2"},
                },
                {
                        name: "aborted",
                        result: &OrchestratorResult{
                                Total:       1,
                                Failed:      1,
                                Aborted:     true,
                                AbortReason: "Foundation package installation failed",
                                Duration:    50 * time.Millisecond,
                        },
                        wantContains: []string{"ABORTED", "Foundation"},
                },
                {
                        name: "rollback",
                        result: &OrchestratorResult{
                                Total:    2,
                                Failed:   1,
                                Aborted:  true,
                                Duration: 50 * time.Millisecond,
                                Rollback: &RollbackReport{
                                        Removed: []string{"git"},
                                        Failed:  []string{"curl"},
                                        Reason:  "Foundation package failure",
                                },
                        },
                        wantContains: []string{"ROLLBACK", "git", "curl", "Foundation package failure"},
                },
                {
                        name: "failed_packages",
                        result: &OrchestratorResult{
                                Total:     2,
                                Succeeded: 1,
                                Failed:    1,
                                Duration:  50 * time.Millisecond,
                                Packages: []PackageResult{
                                        {Package: "git", Action: "install", Success: true},
                                        {Package: "nonexistent", Action: "install", Success: false, Error: "PACKAGE_NOT_FOUND"},
                                },
                        },
                        wantContains: []string{"FAILED PACKAGES", "nonexistent", "PACKAGE_NOT_FOUND"},
                },
        }

        for _, tt := range tests {
                tt := tt
                t.Run(tt.name, func(t *testing.T) {
                        t.Parallel()
                        output := FormatOrchestratorResult(tt.result)
                        for _, want := range tt.wantContains {
                                if !strings.Contains(output, want) {
                                        t.Errorf("FormatOrchestratorResult output missing %q.\nGot:\n%s", want, output)
                                }
                        }
                })
        }
}

func TestOrchestrator_FormatOrchestratorResult_WithPreflight(t *testing.T) {
        t.Parallel()

        result := &OrchestratorResult{
                Total:     1,
                Succeeded: 1,
                Duration:  10 * time.Millisecond,
                Preflight: &PreFlightResult{
                        CanProceed: true,
                        DiskOK:     true,
                        NetworkOK:  true,
                        SudoOK:     true,
                        LockOK:     true,
                },
        }

        output := FormatOrchestratorResult(result)
        if !strings.Contains(output, "Pre-flight") {
                t.Error("output should contain Pre-flight summary")
        }
}

func TestRollbackReport(t *testing.T) {
        t.Parallel()

        report := &RollbackReport{
                Removed: []string{"git", "curl"},
                Failed:  []string{"vim"},
                Reason:  "Foundation package failure",
        }

        if len(report.Removed) != 2 {
                t.Errorf("Removed count = %d, want 2", len(report.Removed))
        }
        if len(report.Failed) != 1 {
                t.Errorf("Failed count = %d, want 1", len(report.Failed))
        }
        if report.Reason != "Foundation package failure" {
                t.Errorf("Reason = %q, want %q", report.Reason, "Foundation package failure")
        }
}

func TestRollbackReport_Empty(t *testing.T) {
        t.Parallel()

        report := &RollbackReport{
                Reason: "no packages to roll back",
        }

        if len(report.Removed) != 0 {
                t.Errorf("Removed count = %d, want 0", len(report.Removed))
        }
        if len(report.Failed) != 0 {
                t.Errorf("Failed count = %d, want 0", len(report.Failed))
        }
}

func TestBoolToResult(t *testing.T) {
        t.Parallel()

        tests := []struct {
                input    bool
                expected string
        }{
                {true, "success"},
                {false, "failure"},
        }

        for _, tt := range tests {
                tt := tt
                t.Run(fmt.Sprintf("%v", tt.input), func(t *testing.T) {
                        t.Parallel()
                        got := boolToResult(tt.input)
                        if got != tt.expected {
                                t.Errorf("boolToResult(%v) = %q, want %q", tt.input, got, tt.expected)
                        }
                })
        }
}

// Test that audit log entries are written during install
func TestOrchestrator_Install_AuditEntries(t *testing.T) {
        tmpDir := t.TempDir()
        origHome := os.Getenv("HOME")
        os.Setenv("HOME", tmpDir)
        defer os.Setenv("HOME", origHome)

        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        audit, err := engine.NewAuditLogger()
        if err != nil {
                t.Fatalf("failed to create AuditLogger: %v", err)
        }
        defer audit.Close()

        state, _ := engine.NewStateTracker()

        o := NewOrchestrator(pm, execFn, state, audit, "dev", false)

        _, installErr := o.Install(context.Background(), []string{"git"})
        if installErr != nil {
                t.Fatalf("Install should succeed: %v", installErr)
        }

        entries, readErr := engine.ReadAuditLog(0)
        if readErr != nil {
                t.Fatalf("failed to read audit log: %v", readErr)
        }
        if len(entries) == 0 {
                t.Error("expected audit log entries, got none")
        }

        foundInstall := false
        for _, e := range entries {
                if e.Action == "install" {
                        foundInstall = true
                        break
                }
        }
        if !foundInstall {
                t.Error("expected at least one 'install' audit entry")
        }
}

// Test that state is recorded for verified packages
func TestOrchestrator_Install_RecordsState(t *testing.T) {
        tmpDir := t.TempDir()
        origHome := os.Getenv("HOME")
        os.Setenv("HOME", tmpDir)
        defer os.Setenv("HOME", origHome)

        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        state, _ := engine.NewStateTracker()

        o := NewOrchestrator(pm, execFn, state, nil, "dev", false)

        _, err := o.Install(context.Background(), []string{"git"})
        if err != nil {
                t.Fatalf("Install should succeed: %v", err)
        }

        if !state.IsManaged("git") {
                t.Error("git should be recorded as managed in state")
        }
}

// Test that the preflight step is included in the result
func TestOrchestrator_Install_PreflightInResult(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        o := NewOrchestrator(pm, execFn, nil, nil, "test", false)

        result, err := o.Install(context.Background(), []string{"git"})
        if err != nil {
                t.Fatalf("Install should succeed: %v", err)
        }

        if result.Preflight == nil {
                t.Error("Preflight should be populated in result")
        }
}

// Verify the audit log file is created in the nexus directory
func TestOrchestrator_AuditFilePath(t *testing.T) {
        tmpDir := t.TempDir()
        origHome := os.Getenv("HOME")
        os.Setenv("HOME", tmpDir)
        defer os.Setenv("HOME", origHome)

        _, err := engine.NewAuditLogger()
        if err != nil {
                t.Fatalf("NewAuditLogger failed: %v", err)
        }

        auditPath := filepath.Join(tmpDir, ".nexus", "audit.log")
        if _, err := os.Stat(auditPath); os.IsNotExist(err) {
                t.Errorf("audit log not found at %s", auditPath)
        }
}

// ---------------------------------------------------------------------------
// Explicit rollback tests — covers the rollback function directly
// ---------------------------------------------------------------------------

// TestOrchestrator_Rollback_EmptyPackages verifies the early return path when
// rollback is called with an empty package list.
func TestOrchestrator_Rollback_EmptyPackages(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()
        o := NewOrchestrator(pm, execFn, nil, nil, "test", false)

        report := o.rollback(context.Background(), []string{})
        if report == nil {
                t.Fatal("rollback should always return a non-nil report")
        }
        if len(report.Removed) != 0 {
                t.Errorf("Removed = %v, want empty", report.Removed)
        }
        if len(report.Failed) != 0 {
                t.Errorf("Failed = %v, want empty", report.Failed)
        }
        if report.Reason == "" {
                t.Error("Reason should not be empty")
        }
}

// TestOrchestrator_Rollback_AllSucceed verifies rollback where PM.Remove
// succeeds for all packages.
func TestOrchestrator_Rollback_AllSucceed(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        // Pre-install packages so Remove can succeed
        pm.installed["git"] = true
        pm.installed["curl"] = true

        audit := newTestAuditLogger(t)
        o := NewOrchestrator(pm, execFn, nil, audit, "dev", false)

        report := o.rollback(context.Background(), []string{"git", "curl"})
        if report == nil {
                t.Fatal("rollback should return a non-nil report")
        }
        if len(report.Removed) != 2 {
                t.Errorf("Removed count = %d, want 2; got %v", len(report.Removed), report.Removed)
        }
        if len(report.Failed) != 0 {
                t.Errorf("Failed count = %d, want 0; got %v", len(report.Failed), report.Failed)
        }
        // Verify packages were actually removed from the mock
        if pm.IsInstalled(context.Background(), "git") {
                t.Error("git should be removed after rollback")
        }
        if pm.IsInstalled(context.Background(), "curl") {
                t.Error("curl should be removed after rollback")
        }
}

// TestOrchestrator_Rollback_PartialFailure verifies rollback where some
// packages fail to remove and some succeed.
func TestOrchestrator_Rollback_PartialFailure(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        // Only git is installed; curl removal will "succeed" (mock treats
        // non-installed packages as successfully removed), but we add
        // an explicit remove error for curl to test partial failure.
        pm.installed["git"] = true
        pm.removeErr["curl"] = fmt.Errorf("cannot remove curl: dependency")

        audit := newTestAuditLogger(t)
        o := NewOrchestrator(pm, execFn, nil, audit, "dev", false)

        report := o.rollback(context.Background(), []string{"git", "curl"})
        if report == nil {
                t.Fatal("rollback should return a non-nil report")
        }
        if len(report.Removed) != 1 {
                t.Errorf("Removed count = %d, want 1; got %v", len(report.Removed), report.Removed)
        }
        if len(report.Failed) != 1 {
                t.Errorf("Failed count = %d, want 1; got %v", len(report.Failed), report.Failed)
        }
        // Verify the right packages are in the right lists
        removedSet := make(map[string]bool)
        for _, p := range report.Removed {
                removedSet[p] = true
        }
        if !removedSet["git"] {
                t.Error("git should be in Removed list")
        }
        failedSet := make(map[string]bool)
        for _, p := range report.Failed {
                failedSet[p] = true
        }
        if !failedSet["curl"] {
                t.Error("curl should be in Failed list")
        }
}

// TestOrchestrator_Rollback_WithStateTracker verifies that the state tracker
// records removals during rollback.
func TestOrchestrator_Rollback_WithStateTracker(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        // Install packages in the mock
        pm.installed["git"] = true
        pm.installed["curl"] = true

        state := newTestStateTracker(t)
        // Record packages in the state tracker so we can verify they're removed
        if err := state.RecordInstall("git", "dev", "apt", true); err != nil {
                t.Fatalf("RecordInstall git failed: %v", err)
        }
        if err := state.RecordInstall("curl", "dev", "apt", true); err != nil {
                t.Fatalf("RecordInstall curl failed: %v", err)
        }

        audit := newTestAuditLogger(t)
        o := NewOrchestrator(pm, execFn, state, audit, "dev", false)

        report := o.rollback(context.Background(), []string{"git", "curl"})
        if report == nil {
                t.Fatal("rollback should return a non-nil report")
        }
        if len(report.Removed) != 2 {
                t.Errorf("Removed count = %d, want 2", len(report.Removed))
        }

        // Verify the state tracker no longer considers these packages managed
        if state.IsManaged("git") {
                t.Error("git should no longer be managed after rollback removal")
        }
        if state.IsManaged("curl") {
                t.Error("curl should no longer be managed after rollback removal")
        }
}

// TestOrchestrator_Rollback_WithStateTracker_NilState verifies rollback
// when state tracker is nil (should not panic).
func TestOrchestrator_Rollback_WithStateTracker_NilState(t *testing.T) {
        pm := newMockPackageManager("apt")
        execFn := simpleMockExec()

        pm.installed["git"] = true

        o := NewOrchestrator(pm, execFn, nil, nil, "dev", false)

        // Should not panic even with nil state tracker
        report := o.rollback(context.Background(), []string{"git"})
        if report == nil {
                t.Fatal("rollback should return a non-nil report")
        }
        if len(report.Removed) != 1 {
                t.Errorf("Removed count = %d, want 1", len(report.Removed))
        }
}
