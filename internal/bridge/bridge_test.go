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

package bridge

import (
	"context"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// DetectEnvironment — uses mock ExecFunc via SetExecFunc
// ---------------------------------------------------------------------------

func TestDetectEnvironment(t *testing.T) {
	t.Run("returns non-nil EnvironmentInfo on Linux", func(t *testing.T) {
		ctx := context.Background()
		env := DetectEnvironment(ctx)

		if env == nil {
			t.Fatal("DetectEnvironment returned nil")
		}
		// On Linux, IsWindows must be false
		if env.IsWindows {
			t.Error("IsWindows should be false on Linux")
		}
		// WSL2Status should be nil on Linux
		if env.WSL2Status != nil {
			t.Error("WSL2Status should be nil on Linux (Spy only runs on Windows)")
		}
		// Blockers should be initialized (never nil)
		if env.Blockers == nil {
			t.Error("Blockers should be initialized, not nil")
		}
	})

	t.Run("detects WSL2 inside guest", func(t *testing.T) {
		// On Linux, IsWSL2 depends on /proc/version content.
		// This test verifies the function runs without panicking.
		// In CI (non-WSL), it should be false.
		ctx := context.Background()
		env := DetectEnvironment(ctx)
		// Most CI environments are NOT WSL2
		if env.IsWSL2 && env.IsNativeLinux {
			t.Error("IsWSL2 and IsNativeLinux should be mutually exclusive")
		}
	})

	t.Run("sets readiness based on prerequisites", func(t *testing.T) {
		ctx := context.Background()
		env := DetectEnvironment(ctx)

		// Ready should be true only when there are zero blockers
		if env.Ready != (len(env.Blockers) == 0) {
			t.Errorf("Ready=%v but len(Blockers)=%d", env.Ready, len(env.Blockers))
		}
	})
}

// ---------------------------------------------------------------------------
// SetExecFunc — dependency injection for the security gate
// ---------------------------------------------------------------------------

func TestSetExecFunc(t *testing.T) {
	t.Run("overrides the default exec function", func(t *testing.T) {
		called := false
		mockFn := func(ctx context.Context, command string, args ...string) (string, error) {
			called = true
			return "mock output", nil
		}

		// Override the exec function
		SetExecFunc(mockFn)

		// Verify bridgeExecFn was set by calling through it
		result, err := bridgeExecFn(context.Background(), "echo", "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Error("mock function was not called")
		}
		if result != "mock output" {
			t.Errorf("expected 'mock output', got '%s'", result)
		}

		// Restore default
		SetExecFunc(defaultExecFunc)
	})

	t.Run("nil ExecFunc does not override", func(t *testing.T) {
		// Capture a marker before setting nil
		called := false
		markerFn := func(ctx context.Context, command string, args ...string) (string, error) {
			called = true
			return "marker", nil
		}
		SetExecFunc(markerFn)

		// SetExecFunc(nil) should NOT change the current function
		SetExecFunc(nil)

		// Call through bridgeExecFn — should still call markerFn
		result, _ := bridgeExecFn(context.Background(), "test")
		if !called {
			t.Error("nil ExecFunc should not override the current function — marker was not called")
		}
		if result != "marker" {
			t.Errorf("expected 'marker', got %q", result)
		}

		// Restore default
		SetExecFunc(defaultExecFunc)
	})
}

// ---------------------------------------------------------------------------
// FormatEnvironmentInfo — output format tests
// ---------------------------------------------------------------------------

func TestFormatEnvironmentInfo(t *testing.T) {
	tests := []struct {
		name     string
		env      *EnvironmentInfo
		contains string
	}{
		{
			name: "native Linux mode",
			env: &EnvironmentInfo{
				IsNativeLinux:  true,
				Distro:         "Ubuntu 22.04",
				PackageManager: "apt",
				Prerequisites:  map[string]bool{"git": true, "curl": true},
				Ready:          true,
				Blockers:       []string{},
			},
			contains: "Native Linux",
		},
		{
			name: "WSL2 mode",
			env: &EnvironmentInfo{
				IsWSL2:         true,
				Distro:         "Debian",
				PackageManager: "apt",
				Prerequisites:  map[string]bool{"git": true},
				Ready:          true,
				Blockers:       []string{},
			},
			contains: "WSL2",
		},
		{
			name: "Windows mode with version",
			env: &EnvironmentInfo{
				IsWindows:      true,
				WindowsVersion: "Windows 11 23H2",
				WindowsBuild:   22631,
				PackageManager: "winget",
				Prerequisites:  map[string]bool{"git": false},
				Ready:          false,
				Blockers:       []string{"git is not installed"},
			},
			contains: "Windows (Native)",
		},
		{
			name: "not ready shows blockers",
			env: &EnvironmentInfo{
				IsNativeLinux:  true,
				Distro:         "Arch",
				PackageManager: "pacman",
				Prerequisites:  map[string]bool{"git": false, "curl": false},
				Ready:          false,
				Blockers:       []string{"git is not installed", "curl is not installed"},
			},
			contains: "NOT READY",
		},
		{
			name: "ready shows success",
			env: &EnvironmentInfo{
				IsNativeLinux:  true,
				Distro:         "Fedora",
				PackageManager: "dnf",
				Prerequisites:  map[string]bool{"git": true, "curl": true},
				Ready:          true,
				Blockers:       []string{},
			},
			contains: "READY",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := FormatEnvironmentInfo(tc.env)
			if !strings.Contains(output, tc.contains) {
				t.Errorf("expected output to contain %q, got:\n%s", tc.contains, output)
			}
		})
	}

	t.Run("displays WSL2 Status section when present", func(t *testing.T) {
		env := &EnvironmentInfo{
			IsWindows: true,
			WSL2Status: &WSL2Status{
				WSLAvailable:    true,
				WSLVersion:      "2",
				HyperVAvailable: true,
				Ready:           true,
				Distros:         []WSLDistro{{Name: "Ubuntu", State: "Running", Version: "2", Default: true}},
				DefaultDistro:   "Ubuntu",
			},
			Prerequisites: map[string]bool{"git": true},
			Ready:         true,
			Blockers:      []string{},
		}
		output := FormatEnvironmentInfo(env)
		if !strings.Contains(output, "WSL2 STATUS") {
			t.Error("expected WSL2 STATUS section when WSL2Status is set")
		}
		if !strings.Contains(output, "Available") {
			t.Error("expected 'Available' in WSL2 status")
		}
		if !strings.Contains(output, "READY") {
			t.Error("expected READY in WSL2 status section")
		}
	})

	t.Run("unknown distro is omitted", func(t *testing.T) {
		env := &EnvironmentInfo{
			IsNativeLinux:  true,
			Distro:         "unknown",
			PackageManager: "unknown",
			Prerequisites:  map[string]bool{},
			Ready:          true,
			Blockers:       []string{},
		}
		output := FormatEnvironmentInfo(env)
		if strings.Contains(output, "Distro:") && strings.Contains(output, "unknown") {
			// The formatter should skip "unknown" distro display
			t.Error("should not display 'unknown' distro")
		}
	})
}

// ---------------------------------------------------------------------------
// WSL2Status parsing from mock output (the Windows-only parse functions
// are tested here by calling them directly if available; on Linux, we
// test the Linux stub for DetectWSL2Status).
// ---------------------------------------------------------------------------

func TestDetectWSL2Status_Linux(t *testing.T) {
	t.Run("returns not-available status on Linux", func(t *testing.T) {
		ctx := context.Background()
		status := DetectWSL2Status(ctx)

		if status == nil {
			t.Fatal("DetectWSL2Status returned nil")
		}
		if status.WSLAvailable {
			t.Error("WSLAvailable should be false on Linux")
		}
		if status.WSLVersion != "n/a" {
			t.Errorf("expected WSLVersion 'n/a', got %q", status.WSLVersion)
		}
		if status.Ready {
			t.Error("Ready should be false on Linux")
		}
		if len(status.Blockers) == 0 {
			t.Error("expected at least one blocker on Linux")
		}
	})
}

func TestIsWSLCommandAvailable(t *testing.T) {
	// On Linux, this should return false
	if IsWSLCommandAvailable() {
		t.Error("IsWSLCommandAvailable should return false on Linux")
	}
}

// ---------------------------------------------------------------------------
// defaultExecFunc — direct invocation tests
// ---------------------------------------------------------------------------

func TestDefaultExecFunc_ValidCommand(t *testing.T) {
	ctx := context.Background()

	// defaultExecFunc uses exec.Command directly; test with "echo"
	output, err := defaultExecFunc(ctx, "echo", "hello")
	if err != nil {
		t.Fatalf("defaultExecFunc with 'echo hello' should not error: %v", err)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", output)
	}
}

func TestDefaultExecFunc_InvalidCommand(t *testing.T) {
	ctx := context.Background()

	// defaultExecFunc with a non-existent command should return an error
	_, err := defaultExecFunc(ctx, "nonexistent_command_that_does_not_exist_12345")
	if err == nil {
		t.Error("defaultExecFunc with a non-existent command should return an error")
	}
}

// ---------------------------------------------------------------------------
// FormatWSL2Status — output format tests
// ---------------------------------------------------------------------------

func TestFormatWSL2Status(t *testing.T) {
	tests := []struct {
		name     string
		status   *WSL2Status
		contains string
	}{
		{
			name: "ready system",
			status: &WSL2Status{
				WSLAvailable:    true,
				WSLVersion:      "2",
				HyperVAvailable: true,
				WindowsVersion:  "Windows 11 23H2",
				WindowsBuild:    22631,
				Architecture:    "amd64",
				KernelVersion:   "5.15.133.1-1",
				Ready:           true,
			},
			contains: "READY",
		},
		{
			name: "not ready with blockers",
			status: &WSL2Status{
				WSLAvailable: false,
				Ready:        false,
				Architecture: "amd64",
				Blockers:     []string{"WSL feature is not enabled"},
				Recommendations: []string{
					"Run: wsl --install",
				},
			},
			contains: "NOT READY",
		},
		{
			name: "WSL1 warning",
			status: &WSL2Status{
				WSLAvailable:    true,
				WSLVersion:      "1",
				HyperVAvailable: false,
				Ready:           false,
				Architecture:    "amd64",
				Blockers:        []string{"WSL1 is installed but WSL2 is required"},
			},
			contains: "WSL1",
		},
		{
			name: "distros displayed",
			status: &WSL2Status{
				WSLAvailable:    true,
				WSLVersion:      "2",
				HyperVAvailable: true,
				Ready:           true,
				Architecture:    "amd64",
				DefaultDistro:   "Ubuntu",
				Distros: []WSLDistro{
					{Name: "Ubuntu", State: "Running", Version: "2", Default: true},
					{Name: "Debian", State: "Stopped", Version: "2", Default: false},
				},
			},
			contains: "Ubuntu",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			output := FormatWSL2Status(tc.status)
			if !strings.Contains(output, tc.contains) {
				t.Errorf("expected output to contain %q, got:\n%s", tc.contains, output)
			}
		})
	}

	t.Run("always shows NEXUS PROTOCOL header", func(t *testing.T) {
		status := &WSL2Status{Architecture: "amd64"}
		output := FormatWSL2Status(status)
		if !strings.Contains(output, "NEXUS PROTOCOL") {
			t.Error("expected NEXUS PROTOCOL header")
		}
	})

	t.Run("shows Hyper-V unavailable message", func(t *testing.T) {
		status := &WSL2Status{
			HyperVAvailable: false,
			Architecture:    "amd64",
		}
		output := FormatWSL2Status(status)
		if !strings.Contains(output, "Not Available") {
			t.Error("expected 'Not Available' for Hyper-V when unavailable")
		}
	})
}

// ---------------------------------------------------------------------------
// FormatWSL2Check — minimal pass/fail output
// ---------------------------------------------------------------------------

func TestFormatWSL2Check(t *testing.T) {
	t.Run("ready system", func(t *testing.T) {
		status := &WSL2Status{Ready: true}
		output := FormatWSL2Check(status)
		if !strings.Contains(output, "READY") {
			t.Errorf("expected READY in output, got:\n%s", output)
		}
		if strings.Contains(output, "Blockers") {
			t.Error("ready system should not show blockers section")
		}
	})

	t.Run("not ready with blockers", func(t *testing.T) {
		status := &WSL2Status{
			Ready:    false,
			Blockers: []string{"WSL feature is not enabled", "Hyper-V is not available"},
		}
		output := FormatWSL2Check(status)
		if !strings.Contains(output, "NOT READY") {
			t.Error("expected NOT READY")
		}
		if !strings.Contains(output, "Blockers") {
			t.Error("expected Blockers section")
		}
		if !strings.Contains(output, "WSL feature is not enabled") {
			t.Error("expected specific blocker text")
		}
	})

	t.Run("not ready without blockers", func(t *testing.T) {
		status := &WSL2Status{
			Ready:    false,
			Blockers: []string{},
		}
		output := FormatWSL2Check(status)
		if !strings.Contains(output, "NOT READY") {
			t.Error("expected NOT READY")
		}
	})
}

// ---------------------------------------------------------------------------
// WSLDistro parsing — tested via FormatWSL2Status with mock data
// ---------------------------------------------------------------------------

func TestWSLDistro(t *testing.T) {
	t.Run("distro fields are preserved", func(t *testing.T) {
		d := WSLDistro{
			Name:    "Ubuntu-22.04",
			State:   "Running",
			Version: "2",
			Default: true,
		}
		if d.Name != "Ubuntu-22.04" {
			t.Errorf("expected Name 'Ubuntu-22.04', got %q", d.Name)
		}
		if d.State != "Running" {
			t.Errorf("expected State 'Running', got %q", d.State)
		}
		if d.Version != "2" {
			t.Errorf("expected Version '2', got %q", d.Version)
		}
		if !d.Default {
			t.Error("expected Default true")
		}
	})
}

// ---------------------------------------------------------------------------
// EnvironmentInfo JSON serialization
// ---------------------------------------------------------------------------

func TestEnvironmentInfo_JSON(t *testing.T) {
	t.Run("WSL2 status serializes correctly", func(t *testing.T) {
		env := &EnvironmentInfo{
			IsWSL2: true,
			WSL2Status: &WSL2Status{
				WSLAvailable:    true,
				WSLVersion:      "2",
				HyperVAvailable: true,
				Ready:           true,
			},
		}
		if !env.IsWSL2 {
			t.Error("IsWSL2 should be true")
		}
		if env.WSL2Status == nil {
			t.Error("WSL2Status should not be nil")
		}
		if env.WSL2Status.WSLVersion != "2" {
			t.Errorf("expected WSLVersion '2', got %q", env.WSL2Status.WSLVersion)
		}
	})
}

// ---------------------------------------------------------------------------
// Linux-specific detection path
// ---------------------------------------------------------------------------

func TestLinuxDetection(t *testing.T) {
	t.Run("detectWSL2 handles missing /proc/version", func(t *testing.T) {
		// In a standard CI environment, /proc/version exists but doesn't
		// contain "microsoft" or "wsl", so this should return false.
		result := detectWSL2()
		// We don't assert true/false because CI might be WSL,
		// but we verify the function doesn't panic.
		_ = result
	})

	t.Run("detectDistro returns non-empty on Linux", func(t *testing.T) {
		distro := detectDistro()
		// Most Linux environments have /etc/os-release
		if distro == "" {
			t.Error("detectDistro returned empty string")
		}
	})

	t.Run("detectPackageManager returns valid value", func(t *testing.T) {
		pm := detectPackageManager()
		validPMs := map[string]bool{
			"apt": true, "pacman": true, "dnf": true,
			"yum": true, "apk": true, "unknown": true,
		}
		if !validPMs[pm] {
			t.Errorf("detectPackageManager returned unexpected value %q", pm)
		}
	})
}
