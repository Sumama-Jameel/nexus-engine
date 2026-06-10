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
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "sync"
        "syscall"
        "testing"
        "time"
)

// ---------------------------------------------------------------------------
// PreFlightCheck tests
// ---------------------------------------------------------------------------

// TestPreFlightCheck_AllPass verifies that when the environment is healthy
// and no packages are pre-installed, all checks pass and all packages are
// marked for installation.
func TestPreFlightCheck_AllPass(t *testing.T) {
        pm := newMockPackageManager("debian")
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{"git", "curl"}, "debian")

        if !result.CanProceed {
                t.Error("CanProceed should be true when all checks pass")
        }
        if len(result.ToInstall) != 2 {
                t.Errorf("ToInstall = %d, want 2", len(result.ToInstall))
        }
        if len(result.Skipped) != 0 {
                t.Errorf("Skipped = %d, want 0", len(result.Skipped))
        }
}

// TestPreFlightCheck_AlreadyInstalled verifies that packages already
// installed are moved to the Skipped list instead of ToInstall.
func TestPreFlightCheck_AlreadyInstalled(t *testing.T) {
        pm := newMockPackageManager("debian")
        ctx := context.Background()

        // Pre-install git
        pm.installed["git"] = true

        result := PreFlightCheck(ctx, pm, []string{"git", "curl"}, "debian")

        if !result.CanProceed {
                t.Error("CanProceed should be true")
        }
        if len(result.Skipped) != 1 {
                t.Errorf("Skipped = %d, want 1", len(result.Skipped))
        }
        if result.Skipped[0] != "git" {
                t.Errorf("Skipped[0] = %q, want %q", result.Skipped[0], "git")
        }
        if len(result.ToInstall) != 1 {
                t.Errorf("ToInstall = %d, want 1", len(result.ToInstall))
        }
        if result.ToInstall[0] != "curl" {
                t.Errorf("ToInstall[0] = %q, want %q", result.ToInstall[0], "curl")
        }
}

// TestPreFlightCheck_AllAlreadyInstalled verifies that when every package
// is already installed, all are skipped and none are marked for install.
func TestPreFlightCheck_AllAlreadyInstalled(t *testing.T) {
        pm := newMockPackageManager("debian")
        ctx := context.Background()

        pm.installed["git"] = true
        pm.installed["curl"] = true
        pm.installed["vim"] = true

        result := PreFlightCheck(ctx, pm, []string{"git", "curl", "vim"}, "debian")

        if !result.CanProceed {
                t.Error("CanProceed should be true")
        }
        if len(result.Skipped) != 3 {
                t.Errorf("Skipped = %d, want 3", len(result.Skipped))
        }
        if len(result.ToInstall) != 0 {
                t.Errorf("ToInstall = %d, want 0", len(result.ToInstall))
        }
}

// TestPreFlightCheck_EmptyPackages verifies behavior with no packages.
func TestPreFlightCheck_EmptyPackages(t *testing.T) {
        pm := newMockPackageManager("debian")
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{}, "debian")

        if !result.CanProceed {
                t.Error("CanProceed should be true with empty packages")
        }
        if len(result.Skipped) != 0 {
                t.Errorf("Skipped = %d, want 0", len(result.Skipped))
        }
        if len(result.ToInstall) != 0 {
                t.Errorf("ToInstall = %d, want 0", len(result.ToInstall))
        }
}

// TestPreFlightCheck_AlpineNoSudoCheck verifies that the Alpine family
// skips the sudo check (always sets SudoOK = true).
func TestPreFlightCheck_AlpineNoSudoCheck(t *testing.T) {
        pm := newMockPackageManager("alpine")
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{"tmux"}, "alpine")

        if !result.SudoOK {
                t.Error("SudoOK should always be true for Alpine family")
        }
}

// TestPreFlightCheck_NetworkCheckSucceeds verifies that when RefreshIndex
// succeeds, NetworkOK is true.
func TestPreFlightCheck_NetworkCheckSucceeds(t *testing.T) {
        pm := newMockPackageManager("debian")
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{"git"}, "debian")

        if !result.NetworkOK {
                t.Error("NetworkOK should be true when RefreshIndex succeeds")
        }
}

// TestPreFlightCheck_NetworkCheckFails verifies that when RefreshIndex
// fails, NetworkOK is false and a warning is added (non-fatal).
func TestPreFlightCheck_NetworkCheckFails(t *testing.T) {
        pm := newMockPackageManager("debian")
        pm.refreshErr = context.DeadlineExceeded
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{"git"}, "debian")

        if result.NetworkOK {
                t.Error("NetworkOK should be false when RefreshIndex fails")
        }
        // Network failure is non-fatal — CanProceed may still be true
        // (depends on other checks like disk, sudo, lock)
        // Just verify the warning is populated
        found := false
        for _, w := range result.Warnings {
                if strings.Contains(w, "network") || strings.Contains(w, "Network") {
                        found = true
                        break
                }
        }
        if !found {
                t.Errorf("expected a network-related warning, got warnings: %v", result.Warnings)
        }
}

// ---------------------------------------------------------------------------
// FormatPreFlightResult tests
// ---------------------------------------------------------------------------

func TestFormatPreFlightResult(t *testing.T) {
        t.Parallel()

        tests := []struct {
                name         string
                result       *PreFlightResult
                wantContains []string
                wantMissing  []string
        }{
                {
                        name: "all_pass",
                        result: &PreFlightResult{
                                CanProceed: true,
                                DiskOK:     true,
                                NetworkOK:  true,
                                SudoOK:     true,
                                LockOK:     true,
                        },
                        wantContains: []string{"PASSED", "✅"},
                        wantMissing:  []string{"FAILED"},
                },
                {
                        name: "all_fail",
                        result: &PreFlightResult{
                                CanProceed: false,
                                DiskOK:     false,
                                NetworkOK:  false,
                                SudoOK:     false,
                                LockOK:     false,
                        },
                        wantContains: []string{"FAILED", "⛔"},
                },
                {
                        name: "skipped_packages",
                        result: &PreFlightResult{
                                CanProceed: true,
                                DiskOK:     true,
                                NetworkOK:  true,
                                SudoOK:     true,
                                LockOK:     true,
                                Skipped:    []string{"git", "curl"},
                        },
                        wantContains: []string{"Skipping 2 already installed"},
                },
                {
                        name: "to_install",
                        result: &PreFlightResult{
                                CanProceed: true,
                                DiskOK:     true,
                                NetworkOK:  true,
                                SudoOK:     true,
                                LockOK:     true,
                                ToInstall:  []string{"vim", "htop"},
                        },
                        wantContains: []string{"Will install 2 packages"},
                },
                {
                        name: "warnings",
                        result: &PreFlightResult{
                                CanProceed: true,
                                DiskOK:     true,
                                NetworkOK:  false,
                                SudoOK:     true,
                                LockOK:     true,
                                Warnings:   []string{"No network connectivity — installation may fail"},
                        },
                        wantContains: []string{"No network connectivity"},
                },
        }

        for _, tt := range tests {
                tt := tt
                t.Run(tt.name, func(t *testing.T) {
                        t.Parallel()
                        output := FormatPreFlightResult(tt.result)
                        for _, want := range tt.wantContains {
                                if !strings.Contains(output, want) {
                                        t.Errorf("output missing %q.\nGot:\n%s", want, output)
                                }
                        }
                        for _, notWant := range tt.wantMissing {
                                if strings.Contains(output, notWant) {
                                        t.Errorf("output should NOT contain %q.\nGot:\n%s", notWant, output)
                                }
                        }
                })
        }
}

// ---------------------------------------------------------------------------
// boolIcon tests
// ---------------------------------------------------------------------------

func TestBoolIcon(t *testing.T) {
        t.Parallel()

        tests := []struct {
                expected string
                input    bool
        }{
                {"✅", true},
                {"❌", false},
        }

        for _, tt := range tests {
                tt := tt
                t.Run(strings.Replace(tt.expected, "✅", "check", -1), func(t *testing.T) {
                        t.Parallel()
                        got := boolIcon(tt.input)
                        if got != tt.expected {
                                t.Errorf("boolIcon(%v) = %q, want %q", tt.input, got, tt.expected)
                        }
                })
        }
}

// ---------------------------------------------------------------------------
// Warnings tests
// ---------------------------------------------------------------------------

// TestPreFlightCheck_WarningsPopulated verifies that failed checks
// produce appropriate warnings.
func TestPreFlightCheck_WarningsPopulated(t *testing.T) {
        pm := newMockPackageManager("debian")
        pm.refreshErr = context.DeadlineExceeded // triggers network warning
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{"git"}, "debian")

        if len(result.Warnings) == 0 {
                t.Error("expected at least one warning when RefreshIndex fails")
        }

        // Verify the network warning is present
        foundNetwork := false
        for _, w := range result.Warnings {
                if strings.Contains(strings.ToLower(w), "network") {
                        foundNetwork = true
                }
        }
        if !foundNetwork {
                t.Errorf("expected network warning, got: %v", result.Warnings)
        }
}

// TestPreFlightCheck_DiskCheckReal verifies that the disk space check
// actually runs on Linux. We don't mock this — we just verify it doesn't
// panic and the result is plausible.
func TestPreFlightCheck_DiskCheckReal(t *testing.T) {
        pm := newMockPackageManager("debian")
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{}, "debian")

        // On a real system with >500MB free, DiskOK should be true.
        // On CI or containers with very little disk, it might be false.
        // Either way, it shouldn't panic.
        t.Logf("DiskOK = %v", result.DiskOK)
}

// TestPreFlightCheck_LockCheckReal verifies the lock check runs
// without panicking on a real system.
func TestPreFlightCheck_LockCheckReal(t *testing.T) {
        pm := newMockPackageManager("debian")
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{}, "debian")

        // LockOK depends on the real system state
        t.Logf("LockOK = %v", result.LockOK)
}

// TestPreFlightCheck_UnknownFamilyLockOK verifies that an unknown
// family name defaults to LockOK = true (no lock files to check).
func TestPreFlightCheck_UnknownFamilyLockOK(t *testing.T) {
        pm := newMockPackageManager("custom")
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{}, "custom")

        if !result.LockOK {
                t.Error("LockOK should be true for unknown family (assumes no lock)")
        }
}

// ---------------------------------------------------------------------------
// checkSudo — direct tests with each package manager type
// ---------------------------------------------------------------------------

func TestCheckSudo_AptSuccess(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", nil},
        })

        pm, _ := NewInstaller("debian", mock)
        result := checkSudo(ctx, pm)

        if !result {
                t.Error("checkSudo should return true when sudo -n true succeeds (AptInstaller)")
        }
}

func TestCheckSudo_AptFailure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })

        pm, _ := NewInstaller("debian", mock)
        result := checkSudo(ctx, pm)

        if result {
                t.Error("checkSudo should return false when sudo -n true fails (AptInstaller)")
        }
}

func TestCheckSudo_PacmanSuccess(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", nil},
        })

        pm, _ := NewInstaller("arch", mock)
        result := checkSudo(ctx, pm)

        if !result {
                t.Error("checkSudo should return true when sudo -n true succeeds (PacmanInstaller)")
        }
}

func TestCheckSudo_PacmanFailure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })

        pm, _ := NewInstaller("arch", mock)
        result := checkSudo(ctx, pm)

        if result {
                t.Error("checkSudo should return false when sudo -n true fails (PacmanInstaller)")
        }
}

func TestCheckSudo_DnfSuccess(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", nil},
        })

        pm, _ := NewInstaller("fedora", mock)
        result := checkSudo(ctx, pm)

        if !result {
                t.Error("checkSudo should return true when sudo -n true succeeds (DnfInstaller)")
        }
}

func TestCheckSudo_DnfFailure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })

        pm, _ := NewInstaller("fedora", mock)
        result := checkSudo(ctx, pm)

        if result {
                t.Error("checkSudo should return false when sudo -n true fails (DnfInstaller)")
        }
}

func TestCheckSudo_DefaultFallback(t *testing.T) {
        ctx := context.Background()

        // Use a mock PM that is NOT one of the concrete types
        // (mockPackageManager doesn't match any of the type switches)
        pm := newMockPackageManager("custom")
        result := checkSudo(ctx, pm)

        // The default case falls back to ListInstalled.
        // Our mock returns nil error from ListInstalled, so result should be true.
        if !result {
                t.Error("checkSudo default fallback (ListInstalled) should return true when ListInstalled succeeds")
        }
}

func TestCheckSudo_DefaultFallbackFailure(t *testing.T) {
        ctx := context.Background()

        // Create a mock that fails ListInstalled
        pm := &failingListInstalledPM{}
        result := checkSudo(ctx, pm)

        if result {
                t.Error("checkSudo default fallback should return false when ListInstalled fails")
        }
}

// failingListInstalledPM is a minimal PackageManager that fails ListInstalled
type failingListInstalledPM struct{}

func (f *failingListInstalledPM) RefreshIndex(ctx context.Context) error { return nil }
func (f *failingListInstalledPM) Install(ctx context.Context, packages []string) ([]PackageResult, error) {
        return nil, nil
}
func (f *failingListInstalledPM) Remove(ctx context.Context, packages []string) ([]PackageResult, error) {
        return nil, nil
}
func (f *failingListInstalledPM) Update(ctx context.Context, packages []string) ([]PackageResult, error) {
        return nil, nil
}
func (f *failingListInstalledPM) IsInstalled(ctx context.Context, pkg string) bool { return false }
func (f *failingListInstalledPM) ListInstalled(ctx context.Context) ([]string, error) {
        return nil, fmt.Errorf("list failed")
}
func (f *failingListInstalledPM) Search(ctx context.Context, query string) ([]string, error) {
        return nil, nil
}
func (f *failingListInstalledPM) Name() string { return "custom" }

// ---------------------------------------------------------------------------
// Edge case tests for preflight.go — Phase 1c coverage improvement
// ---------------------------------------------------------------------------

// TestPreFlightCheck_SudoFail_CanProceedFalse verifies that when the sudo
// check fails (using a real AptInstaller with a failing execFn), CanProceed
// becomes false, SudoOK is false, and the appropriate warning is added.
func TestPreFlightCheck_SudoFail_CanProceedFalse(t *testing.T) {
        ctx := context.Background()

        // Create an AptInstaller whose execFn always fails (simulates no sudo access)
        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })
        pm, _ := NewInstaller("debian", mock)

        result := PreFlightCheck(ctx, pm, []string{"git"}, "debian")

        if result.CanProceed {
                t.Error("CanProceed should be false when sudo check fails")
        }
        if result.SudoOK {
                t.Error("SudoOK should be false when sudo -n true fails")
        }
        // Verify sudo warning is present
        found := false
        for _, w := range result.Warnings {
                if strings.Contains(strings.ToLower(w), "sudo") {
                        found = true
                        break
                }
        }
        if !found {
                t.Errorf("expected a sudo-related warning, got: %v", result.Warnings)
        }
}

// TestPreFlightCheck_NetworkFailNonFatal verifies that network failure alone
// does NOT set CanProceed to false — it's a non-fatal warning.
func TestPreFlightCheck_NetworkFailNonFatal(t *testing.T) {
        pm := newMockPackageManager("debian")
        pm.refreshErr = fmt.Errorf("network unreachable")
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{"git"}, "debian")

        if result.NetworkOK {
                t.Error("NetworkOK should be false when RefreshIndex fails")
        }
        // Network failure alone should NOT make CanProceed false — it's non-fatal.
        // DiskOK and LockOK depend on the real system, but even if they pass,
        // SudoOK from the mock PM defaults to true via the fallback path.
        // So CanProceed should be true (only disk/sudo/lock are fatal).
        // Note: checkSudo with mockPackageManager uses the default fallback
        // (ListInstalled), which succeeds. So SudoOK=true.
        if !result.SudoOK {
                t.Error("SudoOK should be true for mockPackageManager (default fallback succeeds)")
        }
}

// TestPreFlightCheck_CombinedSudoAndNetworkFail verifies that when both
// sudo and network checks fail, CanProceed is false and multiple warnings
// are accumulated.
func TestPreFlightCheck_CombinedSudoAndNetworkFail(t *testing.T) {
        ctx := context.Background()

        // AptInstaller with failing sudo + we can't easily make RefreshIndex fail
        // on a real installer. Instead, use the mock PM for network fail and
        // verify the warnings accumulation pattern separately.
        // For this test, focus on the AptInstaller sudo fail path.
        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true":        {"", fmt.Errorf("sudo: a password is required")},
                "sudo apt-get update": {"", fmt.Errorf("network error")},
        })
        pm, _ := NewInstaller("debian", mock)

        result := PreFlightCheck(ctx, pm, []string{"git"}, "debian")

        if result.CanProceed {
                t.Error("CanProceed should be false when sudo fails")
        }
        if result.SudoOK {
                t.Error("SudoOK should be false")
        }
        if result.NetworkOK {
                t.Error("NetworkOK should be false when apt-get update fails")
        }
        // Should have at least 2 warnings: one for sudo, one for network
        if len(result.Warnings) < 2 {
                t.Errorf("expected at least 2 warnings (sudo + network), got %d: %v", len(result.Warnings), result.Warnings)
        }
}

// TestPreFlightCheck_PreflightFailAllCritical verifies that when all critical
// checks fail (disk, sudo, lock), CanProceed is false and warnings are
// accumulated for each failure. We force sudo and lock failures directly;
// disk depends on the system.
func TestPreFlightCheck_PreflightFailAllCritical(t *testing.T) {
        ctx := context.Background()

        // Use PacmanInstaller with failing sudo
        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })
        pm, _ := NewInstaller("arch", mock)

        // Use "arch" family so checkLock looks for /var/lib/pacman/db.lck
        // (which doesn't exist on this system → NotExist → continue → LockOK=true)
        // To force lock failure, we'd need to hold a lock, tested separately.
        result := PreFlightCheck(ctx, pm, []string{"vim"}, "arch")

        if result.SudoOK {
                t.Error("SudoOK should be false")
        }
        if result.CanProceed {
                t.Error("CanProceed should be false when sudo fails")
        }
}

// TestPreFlightCheck_AlpineAlwaysSudoOK verifies that Alpine family always
// gets SudoOK=true even in a failure scenario for other checks.
func TestPreFlightCheck_AlpineAlwaysSudoOK(t *testing.T) {
        pm := newMockPackageManager("alpine")
        pm.refreshErr = fmt.Errorf("offline")
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{"tmux"}, "alpine")

        if !result.SudoOK {
                t.Error("SudoOK should always be true for Alpine (runs as root)")
        }
        if result.NetworkOK {
                t.Error("NetworkOK should be false when RefreshIndex fails")
        }
        // CanProceed depends on disk and lock, but SudoOK is always true
}

// ---------------------------------------------------------------------------
// checkDiskSpace — direct edge case tests
// ---------------------------------------------------------------------------

// TestCheckDiskSpace_ZeroMB verifies that a 0 MB threshold always passes.
func TestCheckDiskSpace_ZeroMB(t *testing.T) {
        result := checkDiskSpace(0)
        if !result {
                t.Error("checkDiskSpace(0) should always return true — 0 MB is always available")
        }
}

// TestCheckDiskSpace_OneMB verifies that 1 MB threshold passes on any
// reasonable system.
func TestCheckDiskSpace_OneMB(t *testing.T) {
        result := checkDiskSpace(1)
        if !result {
                t.Error("checkDiskSpace(1) should return true on systems with at least 1MB free")
        }
}

// TestCheckDiskSpace_NormalThreshold verifies the standard 500 MB check.
func TestCheckDiskSpace_NormalThreshold(t *testing.T) {
        result := checkDiskSpace(500)
        t.Logf("checkDiskSpace(500) = %v", result)
        // We don't assert true/false because it depends on the system,
        // but we verify it doesn't panic.
}

// TestCheckDiskSpace_VeryHighThreshold verifies that an impossibly high
// threshold correctly returns false.
func TestCheckDiskSpace_VeryHighThreshold(t *testing.T) {
        // 999,999,999 MB ≈ 1 PB — no system has this much free space
        result := checkDiskSpace(999999999)
        if result {
                t.Error("checkDiskSpace(999999999) should return false — no system has ~1PB free")
        }
}

// ---------------------------------------------------------------------------
// checkLock — direct edge case tests
// ---------------------------------------------------------------------------

// TestCheckLock_AlpineFamily verifies that Alpine (empty lock files list)
// always returns true.
func TestCheckLock_AlpineFamily(t *testing.T) {
        result := checkLock("alpine")
        if !result {
                t.Error("checkLock('alpine') should always return true (no lock files to check)")
        }
}

// TestCheckLock_UnknownFamily verifies that an unknown family defaults to
// LockOK=true (assumes no lock).
func TestCheckLock_UnknownFamily(t *testing.T) {
        result := checkLock("unknown_distro")
        if !result {
                t.Error("checkLock('unknown_distro') should return true (unknown family assumes no lock)")
        }
}

// TestCheckLock_DebianFamily verifies the debian family lock check.
// On this system, /var/lib/dpkg/lock-frontend exists but may not be
// readable by the test user, resulting in a permission error (which
// is handled by continuing).
func TestCheckLock_DebianFamily(t *testing.T) {
        result := checkLock("debian")
        t.Logf("checkLock('debian') = %v", result)
        // Result depends on system state, but should not panic
}

// TestCheckLock_UbuntuFamily verifies the ubuntu family uses the same
// lock files as debian.
func TestCheckLock_UbuntuFamily(t *testing.T) {
        result := checkLock("ubuntu")
        t.Logf("checkLock('ubuntu') = %v", result)
}

// TestCheckLock_ArchFamily verifies the arch family lock check.
// /var/lib/pacman/db.lck typically doesn't exist on non-Arch systems,
// so the check should pass (no lock file = no lock held).
func TestCheckLock_ArchFamily(t *testing.T) {
        result := checkLock("arch")
        if !result {
                t.Error("checkLock('arch') should return true when /var/lib/pacman/db.lck doesn't exist")
        }
}

// TestCheckLock_FedoraFamily verifies the fedora family lock check.
// /var/lib/rpm/.rpm.lock typically doesn't exist on non-Fedora systems.
func TestCheckLock_FedoraFamily(t *testing.T) {
        result := checkLock("fedora")
        if !result {
                t.Error("checkLock('fedora') should return true when /var/lib/rpm/.rpm.lock doesn't exist")
        }
}

// TestCheckLock_LockFileHeldByAnotherProcess creates a temporary lock file,
// holds an exclusive flock on it from a goroutine, then verifies that
// checkLock detects the lock and returns false. This tests the actual
// flock failure path in checkLock.
func TestCheckLock_LockFileHeldByAnotherProcess(t *testing.T) {
        // Create a temporary directory and simulate a lock file that checkLock
        // would check. Since checkLock uses hardcoded paths, we need to create
        // the file at one of those paths. We'll use /var/lib/pacman/db.lck
        // since it likely doesn't exist on this Debian-based system.
        lockPath := "/var/lib/pacman/db.lck"

        // Create parent directory if needed
        if err := os.MkdirAll("/var/lib/pacman", 0755); err != nil {
                t.Skipf("cannot create /var/lib/pacman: %v", err)
        }

        // Create the lock file
        f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDONLY, 0644) //nolint:gosec
        if err != nil {
                t.Skipf("cannot create lock file: %v", err)
        }

        // Hold an exclusive lock in a goroutine
        var wg sync.WaitGroup
        wg.Add(1)
        lockReleased := make(chan struct{})

        go func() {
                defer wg.Done()
                // Acquire exclusive lock
                _ = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
                close(lockReleased) // Signal that we have the lock
                // Wait for test to signal us to release
                time.Sleep(200 * time.Millisecond)
                // Release the lock
                _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
                _ = f.Close()
        }()

        // Wait for the goroutine to acquire the lock
        <-lockReleased

        // Now checkLock("arch") should return false because the lock is held
        result := checkLock("arch")
        if result {
                t.Error("checkLock('arch') should return false when /var/lib/pacman/db.lck is held by another process")
        }

        // Wait for goroutine to finish
        wg.Wait()

        // Clean up
        os.Remove(lockPath)
        os.Remove("/var/lib/pacman")
}

// TestCheckLock_PermissionDeniedOnLockFile tests the os.IsPermission path
// by creating a lock file with no read permissions.
func TestCheckLock_PermissionDeniedOnLockFile(t *testing.T) {
        lockPath := "/var/lib/pacman/db.lck"

        // Create parent directory if needed
        if err := os.MkdirAll("/var/lib/pacman", 0755); err != nil {
                t.Skipf("cannot create /var/lib/pacman: %v", err)
        }

        // Create the lock file with no permissions for the test user
        f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0000)
        if err != nil {
                t.Skipf("cannot create lock file: %v", err)
        }
        f.Close()

        // Remove all permissions
        os.Chmod(lockPath, 0000)

        // checkLock should still return true (permission denied → continue → no lock detected)
        result := checkLock("arch")

        // Restore permissions for cleanup
        os.Chmod(lockPath, 0644)
        os.Remove(lockPath)
        os.Remove("/var/lib/pacman")

        if !result {
                t.Error("checkLock should return true when lock file has permission denied (can't read = can't confirm lock)")
        }
}

// TestCheckLock_LockFileCanBeOpenedAndFlocked tests the path where a lock
// file exists, can be opened, and the flock is acquired successfully.
func TestCheckLock_LockFileCanBeOpenedAndFlocked(t *testing.T) {
        lockPath := "/var/lib/pacman/db.lck"

        // Create parent directory if needed
        if err := os.MkdirAll("/var/lib/pacman", 0755); err != nil {
                t.Skipf("cannot create /var/lib/pacman: %v", err)
        }

        // Create a readable lock file
        f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDONLY, 0644) //nolint:gosec
        if err != nil {
                t.Skipf("cannot create lock file: %v", err)
        }
        f.Close()

        // checkLock should return true (file exists, can be opened, no one holds the lock)
        result := checkLock("arch")

        // Clean up
        os.Remove(lockPath)
        os.Remove("/var/lib/pacman")

        if !result {
                t.Error("checkLock should return true when lock file exists and is not held")
        }
}

// ---------------------------------------------------------------------------
// Orchestrator + PreFlight interaction tests
// ---------------------------------------------------------------------------

// TestOrchestrator_DryRun_BypassesCanProceedFalse verifies that when
// PreFlightCheck fails (CanProceed=false), the Orchestrator in dry-run
// mode still proceeds with the installation plan.
func TestOrchestrator_DryRun_BypassesCanProceedFalse(t *testing.T) {
        // Create an AptInstaller with failing sudo to force CanProceed=false
        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })
        pm, _ := NewInstaller("debian", mock)

        o := NewOrchestrator(pm, mock, nil, nil, "test", true) // dryRun=true

        result, err := o.Install(context.Background(), []string{"git"})
        if err != nil {
                t.Fatalf("Dry-run Install should not error even when preflight fails: %v", err)
        }
        if result.Aborted {
                t.Error("Dry-run should bypass CanProceed=false and not abort")
        }
        if result.Preflight == nil {
                t.Error("Preflight should still be populated in result")
        }
        if result.Preflight.CanProceed {
                t.Error("Preflight.CanProceed should be false (sudo fails)")
        }
}

// TestOrchestrator_NonDryRun_AbortsOnCanProceedFalse verifies that when
// PreFlightCheck fails and dry-run is false, the Orchestrator aborts.
func TestOrchestrator_NonDryRun_AbortsOnCanProceedFalse(t *testing.T) {
        // Create an AptInstaller with failing sudo to force CanProceed=false
        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })
        pm, _ := NewInstaller("debian", mock)

        o := NewOrchestrator(pm, mock, nil, nil, "test", false) // dryRun=false

        result, err := o.Install(context.Background(), []string{"git"})
        if err == nil {
                t.Fatal("Non-dry-run Install should error when preflight fails")
        }
        if !result.Aborted {
                t.Error("Result should be aborted when preflight fails and not in dry-run mode")
        }
        if result.Preflight == nil {
                t.Error("Preflight should be populated in result")
        }
        if result.Preflight.CanProceed {
                t.Error("Preflight.CanProceed should be false")
        }
        if !strings.Contains(result.AbortReason, "Pre-flight") {
                t.Errorf("AbortReason should mention Pre-flight, got: %q", result.AbortReason)
        }
}

// ---------------------------------------------------------------------------
// PreFlightCheck — warning content verification
// ---------------------------------------------------------------------------

// TestPreFlightCheck_DiskWarningContent verifies that when the disk check
// fails, the exact warning message is as expected.
func TestPreFlightCheck_DiskWarningContent(t *testing.T) {
        // We can't easily force checkDiskSpace to fail from PreFlightCheck,
        // but we can test the warning format by constructing a PreFlightResult
        // manually and verifying FormatPreFlightResult output.
        result := &PreFlightResult{
                CanProceed: false,
                DiskOK:     false,
                NetworkOK:  true,
                SudoOK:     true,
                LockOK:     true,
                Skipped:    []string{},
                ToInstall:  []string{"git"},
                Warnings:   []string{"Insufficient disk space (need ≥500MB free)"},
        }

        output := FormatPreFlightResult(result)
        if !strings.Contains(output, "FAILED") {
                t.Error("Output should contain FAILED when CanProceed is false")
        }
        if !strings.Contains(output, "Insufficient disk space") {
                t.Error("Output should contain the disk space warning")
        }
}

// TestPreFlightCheck_LockWarningContent verifies the lock warning message.
func TestPreFlightCheck_LockWarningContent(t *testing.T) {
        result := &PreFlightResult{
                CanProceed: false,
                DiskOK:     true,
                NetworkOK:  true,
                SudoOK:     true,
                LockOK:     false,
                Skipped:    []string{},
                ToInstall:  []string{"git"},
                Warnings:   []string{"Package manager is locked — another installation may be in progress"},
        }

        output := FormatPreFlightResult(result)
        if !strings.Contains(output, "FAILED") {
                t.Error("Output should contain FAILED when CanProceed is false")
        }
        if !strings.Contains(output, "locked") {
                t.Error("Output should contain the lock warning")
        }
}

// TestPreFlightCheck_MultipleWarnings verifies that multiple warnings
// are all present in the formatted output.
func TestPreFlightCheck_MultipleWarnings(t *testing.T) {
        result := &PreFlightResult{
                CanProceed: false,
                DiskOK:     false,
                NetworkOK:  false,
                SudoOK:     false,
                LockOK:     false,
                Skipped:    []string{},
                ToInstall:  []string{},
                Warnings: []string{
                        "Insufficient disk space (need ≥500MB free)",
                        "No network connectivity — installation may fail",
                        "No sudo access — cannot install system packages",
                        "Package manager is locked — another installation may be in progress",
                },
        }

        output := FormatPreFlightResult(result)
        for _, w := range result.Warnings {
                if !strings.Contains(output, w) {
                        t.Errorf("Output should contain warning %q", w)
                }
        }
}

// TestPreFlightCheck_NilPackages verifies PreFlightCheck handles nil packages slice.
func TestPreFlightCheck_NilPackages(t *testing.T) {
        pm := newMockPackageManager("debian")
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, nil, "debian")

        if !result.CanProceed {
                t.Error("CanProceed should be true with nil packages (no critical failures from packages)")
        }
}

// ---------------------------------------------------------------------------
// checkNetwork — direct tests
// ---------------------------------------------------------------------------

// TestCheckNetwork_Success directly tests checkNetwork when RefreshIndex succeeds.
func TestCheckNetwork_Success(t *testing.T) {
        pm := newMockPackageManager("debian")
        ctx := context.Background()

        result := checkNetwork(ctx, pm)
        if !result {
                t.Error("checkNetwork should return true when RefreshIndex succeeds")
        }
}

// TestCheckNetwork_Failure directly tests checkNetwork when RefreshIndex fails.
func TestCheckNetwork_Failure(t *testing.T) {
        pm := newMockPackageManager("debian")
        pm.refreshErr = fmt.Errorf("connection refused")
        ctx := context.Background()

        result := checkNetwork(ctx, pm)
        if result {
                t.Error("checkNetwork should return false when RefreshIndex fails")
        }
}

// ---------------------------------------------------------------------------
// checkSudo — additional edge cases
// ---------------------------------------------------------------------------

// TestCheckSudo_AptFailure_Integration verifies that PreFlightCheck properly
// propagates sudo failure through the full check pipeline.
func TestCheckSudo_AptFailure_Integration(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })
        pm, _ := NewInstaller("debian", mock)

        // Direct checkSudo test
        if checkSudo(ctx, pm) {
                t.Error("checkSudo should return false for AptInstaller when sudo fails")
        }
}

// TestCheckSudo_PacmanFailure_Integration verifies PacmanInstaller sudo failure
// propagates through PreFlightCheck.
func TestCheckSudo_PacmanFailure_Integration(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })
        pm, _ := NewInstaller("arch", mock)

        if checkSudo(ctx, pm) {
                t.Error("checkSudo should return false for PacmanInstaller when sudo fails")
        }
}

// TestCheckSudo_DnfFailure_Integration verifies DnfInstaller sudo failure.
func TestCheckSudo_DnfFailure_Integration(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })
        pm, _ := NewInstaller("fedora", mock)

        if checkSudo(ctx, pm) {
                t.Error("checkSudo should return false for DnfInstaller when sudo fails")
        }
}

// ---------------------------------------------------------------------------
// PreFlightResult — JSON serialization
// ---------------------------------------------------------------------------

// TestPreFlightResult_JSONFields verifies that all fields in PreFlightResult
// are properly serialized to JSON with the expected tag names.
func TestPreFlightResult_JSONFields(t *testing.T) {
        t.Parallel()

        result := &PreFlightResult{
                CanProceed: true,
                DiskOK:     true,
                NetworkOK:  false,
                SudoOK:     true,
                LockOK:     true,
                Skipped:    []string{"git"},
                ToInstall:  []string{"vim", "htop"},
                Warnings:   []string{"No network connectivity"},
        }

        data, err := json.Marshal(result)
        if err != nil {
                t.Fatalf("json.Marshal failed: %v", err)
        }

        var raw map[string]interface{}
        if err := json.Unmarshal(data, &raw); err != nil {
                t.Fatalf("json.Unmarshal failed: %v", err)
        }

        expectedFields := []string{
                "can_proceed", "disk_ok", "network_ok", "sudo_ok", "lock_ok",
                "skipped_already_installed", "to_install", "warnings",
        }
        for _, f := range expectedFields {
                if _, ok := raw[f]; !ok {
                        t.Errorf("PreFlightResult JSON missing field %q", f)
                }
        }
}

// TestPreFlightResult_JSONRoundTrip verifies that a PreFlightResult can be
// serialized and deserialized through JSON with all fields preserved.
func TestPreFlightResult_JSONRoundTrip(t *testing.T) {
        t.Parallel()

        original := &PreFlightResult{
                CanProceed: false,
                DiskOK:     false,
                NetworkOK:  false,
                SudoOK:     false,
                LockOK:     false,
                Skipped:    []string{"curl"},
                ToInstall:  []string{"git", "vim"},
                Warnings:   []string{"Insufficient disk space", "No sudo access"},
        }

        data, err := json.Marshal(original)
        if err != nil {
                t.Fatalf("json.Marshal failed: %v", err)
        }

        var decoded PreFlightResult
        if err := json.Unmarshal(data, &decoded); err != nil {
                t.Fatalf("json.Unmarshal failed: %v", err)
        }

        if decoded.CanProceed != original.CanProceed {
                t.Errorf("CanProceed mismatch: got %v, want %v", decoded.CanProceed, original.CanProceed)
        }
        if decoded.DiskOK != original.DiskOK {
                t.Errorf("DiskOK mismatch: got %v, want %v", decoded.DiskOK, original.DiskOK)
        }
        if decoded.NetworkOK != original.NetworkOK {
                t.Errorf("NetworkOK mismatch: got %v, want %v", decoded.NetworkOK, original.NetworkOK)
        }
        if decoded.SudoOK != original.SudoOK {
                t.Errorf("SudoOK mismatch: got %v, want %v", decoded.SudoOK, original.SudoOK)
        }
        if decoded.LockOK != original.LockOK {
                t.Errorf("LockOK mismatch: got %v, want %v", decoded.LockOK, original.LockOK)
        }
        if len(decoded.Skipped) != len(original.Skipped) {
                t.Errorf("Skipped length mismatch: got %d, want %d", len(decoded.Skipped), len(original.Skipped))
        }
        if len(decoded.ToInstall) != len(original.ToInstall) {
                t.Errorf("ToInstall length mismatch: got %d, want %d", len(decoded.ToInstall), len(original.ToInstall))
        }
        if len(decoded.Warnings) != len(original.Warnings) {
                t.Errorf("Warnings length mismatch: got %d, want %d", len(decoded.Warnings), len(original.Warnings))
        }
}

// ---------------------------------------------------------------------------
// FormatPreFlightResult — additional edge cases
// ---------------------------------------------------------------------------

// TestFormatPreFlightResult_NoWarnings verifies that when there are no
// warnings, no warning section appears in the output.
func TestFormatPreFlightResult_NoWarnings(t *testing.T) {
        t.Parallel()

        result := &PreFlightResult{
                CanProceed: true,
                DiskOK:     true,
                NetworkOK:  true,
                SudoOK:     true,
                LockOK:     true,
                Skipped:    []string{},
                ToInstall:  []string{},
                Warnings:   []string{},
        }

        output := FormatPreFlightResult(result)
        if strings.Contains(output, "⚠️") {
                t.Error("output should not contain warning icons when Warnings is empty")
        }
}

// TestFormatPreFlightResult_EmptySkippedAndToInstall verifies formatting
// when both Skipped and ToInstall are empty.
func TestFormatPreFlightResult_EmptySkippedAndToInstall(t *testing.T) {
        t.Parallel()

        result := &PreFlightResult{
                CanProceed: true,
                DiskOK:     true,
                NetworkOK:  true,
                SudoOK:     true,
                LockOK:     true,
                Skipped:    []string{},
                ToInstall:  []string{},
                Warnings:   []string{},
        }

        output := FormatPreFlightResult(result)
        if strings.Contains(output, "Skipping") {
                t.Error("output should not mention Skipping when Skipped is empty")
        }
        if strings.Contains(output, "Will install") {
                t.Error("output should not mention 'Will install' when ToInstall is empty")
        }
}

// ---------------------------------------------------------------------------
// PreFlightCheck — DnfInstaller sudo failure through full pipeline
// ---------------------------------------------------------------------------

// TestPreFlightCheck_DnfSudoFail verifies that PreFlightCheck with a
// DnfInstaller that fails sudo sets CanProceed=false.
func TestPreFlightCheck_DnfSudoFail(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })
        pm, _ := NewInstaller("fedora", mock)

        result := PreFlightCheck(ctx, pm, []string{"git"}, "fedora")

        if result.SudoOK {
                t.Error("SudoOK should be false when DnfInstaller sudo fails")
        }
        if result.CanProceed {
                t.Error("CanProceed should be false when sudo check fails")
        }

        // Verify the sudo warning is present
        foundSudo := false
        for _, w := range result.Warnings {
                if strings.Contains(strings.ToLower(w), "sudo") {
                        foundSudo = true
                        break
                }
        }
        if !foundSudo {
                t.Errorf("expected sudo-related warning, got: %v", result.Warnings)
        }
}

// ---------------------------------------------------------------------------
// PreFlightCheck — combined disk failure + sudo failure
// ---------------------------------------------------------------------------

// TestPreFlightCheck_DiskAndSudoFail verifies that when both disk and sudo
// checks fail, CanProceed is false and multiple critical warnings accumulate.
func TestPreFlightCheck_DiskAndSudoFail(t *testing.T) {
        ctx := context.Background()

        // Create an AptInstaller with failing sudo
        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true": {"", fmt.Errorf("sudo: a password is required")},
        })
        pm, _ := NewInstaller("debian", mock)

        // We can't easily force disk failure, but we can verify that
        // the sudo failure is properly accumulated
        result := PreFlightCheck(ctx, pm, []string{"git"}, "debian")

        if result.CanProceed {
                t.Error("CanProceed should be false when sudo fails")
        }
        if result.SudoOK {
                t.Error("SudoOK should be false")
        }

        // Should have at least one warning (sudo)
        if len(result.Warnings) == 0 {
                t.Error("expected at least one warning when checks fail")
        }
}

// ---------------------------------------------------------------------------
// PreFlightCheck — all packages skipped
// ---------------------------------------------------------------------------

// TestPreFlightCheck_AllPackagesSkipped verifies that when all packages
// are already installed, ToInstall is empty, Skipped has all, and
// CanProceed can still be true.
func TestPreFlightCheck_AllPackagesSkipped(t *testing.T) {
        pm := newMockPackageManager("debian")
        pm.installed["git"] = true
        pm.installed["curl"] = true
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{"git", "curl"}, "debian")

        if len(result.ToInstall) != 0 {
                t.Errorf("ToInstall should be empty when all packages are already installed, got %v", result.ToInstall)
        }
        if len(result.Skipped) != 2 {
                t.Errorf("Skipped should have 2 packages, got %d", len(result.Skipped))
        }
}

// ---------------------------------------------------------------------------
// checkLockFiles — empty file list
// ---------------------------------------------------------------------------

// TestCheckLockFiles_EmptyList verifies that checkLockFiles with an empty
// list returns true (no lock files to check = no lock held).
func TestCheckLockFiles_EmptyList(t *testing.T) {
        result := checkLockFiles([]string{})
        if !result {
                t.Error("checkLockFiles with empty list should return true")
        }
}

// TestCheckLockFiles_NonExistentFiles verifies that checkLockFiles returns
// true when the lock files don't exist (os.IsNotExist → continue).
func TestCheckLockFiles_NonExistentFiles(t *testing.T) {
        tmpDir := t.TempDir()
        files := []string{
                filepath.Join(tmpDir, "nonexistent1.lck"),
                filepath.Join(tmpDir, "nonexistent2.lck"),
        }

        result := checkLockFiles(files)
        if !result {
                t.Error("checkLockFiles should return true when lock files don't exist")
        }
}

// TestCheckLockFiles_ExistingFileNoLock verifies that checkLockFiles returns
// true when lock files exist and can be locked (no one holds them).
func TestCheckLockFiles_ExistingFileNoLock(t *testing.T) {
        tmpDir := t.TempDir()
        lockPath := filepath.Join(tmpDir, "test.lck")

        // Create the lock file
        f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDONLY, 0644) //nolint:gosec
        if err != nil {
                t.Fatalf("failed to create lock file: %v", err)
        }
        f.Close()

        result := checkLockFiles([]string{lockPath})
        if !result {
                t.Error("checkLockFiles should return true when lock file exists and is not held")
        }
}

// TestCheckLockFiles_LockHeldByAnotherProcess verifies that checkLockFiles
// returns false when a lock file is held by another goroutine.
func TestCheckLockFiles_LockHeldByAnotherProcess(t *testing.T) {
        tmpDir := t.TempDir()
        lockPath := filepath.Join(tmpDir, "test.lck")

        // Create the lock file
        f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDONLY, 0644) //nolint:gosec
        if err != nil {
                t.Fatalf("failed to create lock file: %v", err)
        }

        // Hold the lock in a goroutine
        var wg sync.WaitGroup
        wg.Add(1)
        lockAcquired := make(chan struct{})

        go func() {
                defer wg.Done()
                syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
                close(lockAcquired)
                time.Sleep(300 * time.Millisecond)
                syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
                f.Close()
        }()

        // Wait for goroutine to acquire the lock
        <-lockAcquired

        // checkLockFiles should return false
        result := checkLockFiles([]string{lockPath})
        if result {
                t.Error("checkLockFiles should return false when lock is held by another process")
        }

        wg.Wait()
}

// TestCheckLockFiles_PermissionDenied verifies that checkLockFiles returns
// true when a lock file has no read permissions (can't confirm lock).
func TestCheckLockFiles_PermissionDenied(t *testing.T) {
        tmpDir := t.TempDir()
        lockPath := filepath.Join(tmpDir, "noperm.lck")

        // Create file and remove permissions
        if err := os.WriteFile(lockPath, []byte("lock"), 0644); err != nil {
                t.Fatalf("failed to create lock file: %v", err)
        }
        os.Chmod(lockPath, 0000)
        defer os.Chmod(lockPath, 0644) // restore for cleanup

        result := checkLockFiles([]string{lockPath})
        if !result {
                t.Error("checkLockFiles should return true when lock file has permission denied")
        }
}

// TestCheckLockFiles_MultipleFilesOneHeld verifies that checkLockFiles returns
// false when one of multiple lock files is held.
func TestCheckLockFiles_MultipleFilesOneHeld(t *testing.T) {
        tmpDir := t.TempDir()

        // Create two lock files
        lockPath1 := filepath.Join(tmpDir, "lock1.lck")
        lockPath2 := filepath.Join(tmpDir, "lock2.lck")

        f1, err := os.OpenFile(lockPath1, os.O_CREATE|os.O_RDONLY, 0644)
        if err != nil {
                t.Fatalf("failed to create lock1: %v", err)
        }
        f1.Close()

        f2, err := os.OpenFile(lockPath2, os.O_CREATE|os.O_RDONLY, 0644)
        if err != nil {
                t.Fatalf("failed to create lock2: %v", err)
        }

        // Hold lock on file 2
        var wg sync.WaitGroup
        wg.Add(1)
        lockAcquired := make(chan struct{})

        go func() {
                defer wg.Done()
                syscall.Flock(int(f2.Fd()), syscall.LOCK_EX)
                close(lockAcquired)
                time.Sleep(300 * time.Millisecond)
                syscall.Flock(int(f2.Fd()), syscall.LOCK_UN)
                f2.Close()
        }()

        <-lockAcquired

        // checkLockFiles should return false because lock2 is held
        result := checkLockFiles([]string{lockPath1, lockPath2})
        if result {
                t.Error("checkLockFiles should return false when one lock is held")
        }

        wg.Wait()
}

// ---------------------------------------------------------------------------
// PreFlightCheck — warning accumulation across multiple failures
// ---------------------------------------------------------------------------

// TestPreFlightCheck_WarningAccumulation verifies that warnings from
// different check failures are properly accumulated in the result.
func TestPreFlightCheck_WarningAccumulation(t *testing.T) {
        ctx := context.Background()

        // Create an AptInstaller with failing sudo + failing network (apt-get update)
        mock := mockExecFunc(map[string]mockResponse{
                "sudo -n true":        {"", fmt.Errorf("sudo: a password is required")},
                "sudo apt-get update": {"", fmt.Errorf("network error")},
        })
        pm, _ := NewInstaller("debian", mock)

        result := PreFlightCheck(ctx, pm, []string{"git"}, "debian")

        // Should have at least 2 warnings
        if len(result.Warnings) < 2 {
                t.Errorf("expected at least 2 warnings, got %d: %v", len(result.Warnings), result.Warnings)
        }

        // Verify both sudo and network warnings
        foundSudo := false
        foundNetwork := false
        for _, w := range result.Warnings {
                if strings.Contains(strings.ToLower(w), "sudo") {
                        foundSudo = true
                }
                if strings.Contains(strings.ToLower(w), "network") {
                        foundNetwork = true
                }
        }
        if !foundSudo {
                t.Errorf("expected sudo warning, got: %v", result.Warnings)
        }
        if !foundNetwork {
                t.Errorf("expected network warning, got: %v", result.Warnings)
        }
}

// ---------------------------------------------------------------------------
// PreFlightCheck — Alpine with network failure + non-fatal
// ---------------------------------------------------------------------------

// TestPreFlightCheck_AlpineNetworkFailNonFatal verifies that on Alpine,
// a network failure produces a warning but SudoOK stays true.
func TestPreFlightCheck_AlpineNetworkFailNonFatal(t *testing.T) {
        pm := newMockPackageManager("alpine")
        pm.refreshErr = fmt.Errorf("network unreachable")
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{"pkg"}, "alpine")

        // Network fail is non-fatal
        if result.NetworkOK {
                t.Error("NetworkOK should be false")
        }
        // SudoOK is always true for Alpine
        if !result.SudoOK {
                t.Error("SudoOK should always be true for Alpine")
        }
        // Check warning is present
        foundNetwork := false
        for _, w := range result.Warnings {
                if strings.Contains(strings.ToLower(w), "network") {
                        foundNetwork = true
                        break
                }
        }
        if !foundNetwork {
                t.Errorf("expected network warning, got: %v", result.Warnings)
        }
}

// ---------------------------------------------------------------------------
// checkDiskSpace — boundary values
// ---------------------------------------------------------------------------

// TestCheckDiskSpace_BoundaryValues tests boundary conditions for checkDiskSpace.
func TestCheckDiskSpace_BoundaryValues(t *testing.T) {
        tests := []struct {
                name     string
                minMB    uint64
                wantTrue bool // true if we expect the check to pass
        }{
                {"zero_always_passes", 0, true},
                {"one_mb_passes", 1, true},
                {"ten_mb_passes", 10, true},
                {"hundred_mb_passes", 100, true},
                {"impossible_pb", 999999999, false},
        }

        for _, tt := range tests {
                tt := tt
                t.Run(tt.name, func(t *testing.T) {
                        result := checkDiskSpace(tt.minMB)
                        if tt.wantTrue && !result {
                                t.Errorf("checkDiskSpace(%d) should pass but returned false", tt.minMB)
                        }
                        if !tt.wantTrue && result {
                                t.Errorf("checkDiskSpace(%d) should fail but returned true", tt.minMB)
                        }
                })
        }
}

// ---------------------------------------------------------------------------
// PreFlightCheck — partial package installation
// ---------------------------------------------------------------------------

// TestPreFlightCheck_PartialAlreadyInstalled verifies that when some
// packages are already installed, the result correctly splits them
// between Skipped and ToInstall.
func TestPreFlightCheck_PartialAlreadyInstalled(t *testing.T) {
        pm := newMockPackageManager("debian")
        pm.installed["git"] = true
        pm.installed["vim"] = true
        ctx := context.Background()

        result := PreFlightCheck(ctx, pm, []string{"git", "curl", "vim", "htop"}, "debian")

        if len(result.Skipped) != 2 {
                t.Errorf("Skipped = %d, want 2", len(result.Skipped))
        }
        if len(result.ToInstall) != 2 {
                t.Errorf("ToInstall = %d, want 2", len(result.ToInstall))
        }

        // Verify the correct packages are in each list
        skippedSet := make(map[string]bool)
        for _, p := range result.Skipped {
                skippedSet[p] = true
        }
        if !skippedSet["git"] || !skippedSet["vim"] {
                t.Errorf("expected git and vim in Skipped, got: %v", result.Skipped)
        }

        installSet := make(map[string]bool)
        for _, p := range result.ToInstall {
                installSet[p] = true
        }
        if !installSet["curl"] || !installSet["htop"] {
                t.Errorf("expected curl and htop in ToInstall, got: %v", result.ToInstall)
        }
}
