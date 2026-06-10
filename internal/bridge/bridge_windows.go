//go:build windows

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
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// detectEnvironmentImpl probes the Windows environment.
// This file is compiled ONLY on Windows (//go:build windows).
//
// Per V4 "The WSL2 Detector (The Spy)":
// "A Windows .exe version of your Go tool that checks if WSL2 is enabled."
// "Query Windows Registry or run wsl --status."
//
// This function detects:
// - Windows version and build number (via Registry)
// - Whether WSL2 is available (via Registry + wsl command)
// - Whether Hyper-V is present (prerequisite for WSL2)
// - Available Windows package managers (winget, scoop, choco)
//
// IMPORTANT: All command execution goes through bridgeExecFn (the injected
// ExecFunc), NOT through raw exec.Command. Per the Nexus Protocol Zero-Trust
// Architecture: "Every system call the Go engine makes to the shell must pass
// through a centralized SanitizeAndExecute function."
func detectEnvironmentImpl(ctx context.Context) *EnvironmentInfo {
	env := &EnvironmentInfo{
		IsWindows:     true,
		IsNativeLinux: false,
		IsWSL2:        false, // Never WSL2 when running natively on Windows
	}

	// Detect Windows version via Registry
	env.WindowsVersion, env.WindowsBuild = detectWindowsVersion()

	// Detect WSL availability from the Windows side
	// (This is the CORE V4 feature: detecting WSL2 FROM Windows)
	// CRITICAL FIX: Store the WSL2 status in the EnvironmentInfo struct
	// so that `nexus probe` on Windows shows WSL2 information.
	env.WSL2Status = detectWSLAvailability(ctx)

	// Detect Windows package managers
	env.PackageManager = detectWindowsPackageManager(ctx)

	_ = runtime.GOOS // Already guaranteed to be "windows" by build tag

	return env
}

// detectWindowsVersion reads the Windows version from the Registry.
// Uses HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion.
//
// Per the V4 plan: we use golang.org/x/sys/windows/registry for
// safe, read-only Registry access via Windows API (RegOpenKeyEx,
// RegQueryValueEx) — not shell commands.
func detectWindowsVersion() (version string, build int) {
	// Try Registry-based detection first (most reliable)
	version, build = detectWindowsVersionFromRegistry()
	if version != "" {
		return version, build
	}

	// Fallback: use `ver` command (available on all Windows versions)
	return detectWindowsVersionFromCommand()
}

// detectWindowsVersionFromRegistry reads version info from the Windows Registry.
// This is the PRIMARY detection method — it uses Windows API calls,
// not shell commands, eliminating injection risks.
func detectWindowsVersionFromRegistry() (string, int) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion`,
		registry.QUERY_VALUE)
	if err != nil {
		return "", 0
	}
	defer key.Close()

	productName, _, err := key.GetStringValue("ProductName")
	if err != nil {
		productName = "Windows"
	}

	// DisplayVersion is present on Windows 10 20H2+ (e.g., "22H2")
	displayVersion, _, _ := key.GetStringValue("DisplayVersion")

	// CurrentBuild is the build number (e.g., "19045")
	buildStr, _, err := key.GetStringValue("CurrentBuild")
	build := 0
	if err == nil {
		build, _ = strconv.Atoi(buildStr)
	}

	// Also try CurrentBuildNumber (newer Windows versions)
	if build == 0 {
		buildStr, _, err = key.GetStringValue("CurrentBuildNumber")
		if err == nil {
			build, _ = strconv.Atoi(buildStr)
		}
	}

	// Compose version string
	version := productName
	if displayVersion != "" {
		version = productName + " " + displayVersion
	}

	return version, build
}

// detectWindowsVersionFromCommand is a fallback that uses the `ver` command.
// Used when Registry access fails (e.g., insufficient permissions).
//
// Per Zero-Trust: routes through bridgeExecFn (SanitizeAndExecute),
// NOT through raw exec.Command.
func detectWindowsVersionFromCommand() (string, int) {
	output, err := bridgeExecFn(context.Background(), "cmd", "/c", "ver")
	if err != nil {
		return "Windows (Unknown Version)", 0
	}

	// Parse output like: "Microsoft Windows [Version 10.0.19045.3803]"
	content := output
	version := "Windows"

	if idx := strings.Index(content, "[Version "); idx != -1 {
		start := idx + len("[Version ")
		end := strings.Index(content[start:], "]")
		if end > 0 {
			fullVersion := content[start : start+end]
			parts := strings.SplitN(fullVersion, ".", 4)
			if len(parts) >= 3 {
				build, _ := strconv.Atoi(parts[2])
				major, _ := strconv.Atoi(parts[0])
				if major >= 10 && build >= 22000 {
					version = "Windows 11"
				} else if major >= 10 {
					version = "Windows 10"
				}
				return version, build
			}
		}
	}

	return version, 0
}

// detectWSLAvailability checks if WSL2 is available on this Windows system.
// This is the V4 "Spy" functionality — detecting the WSL2 state FROM Windows.
//
// Uses two methods:
// 1. Registry: Check HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Lxss
// 2. Command: Run `wsl --status` and parse output (via bridgeExecFn)
func detectWSLAvailability(ctx context.Context) *WSL2Status {
	status := &WSL2Status{
		Architecture: runtime.GOARCH,
		Blockers:     []string{},
	}

	// Step 1: Check Windows build (WSL2 requires build 19041+)
	status.WindowsBuild = detectWindowsBuild()
	status.WindowsVersion = detectWindowsProductName()

	if status.WindowsBuild < 19041 && status.WindowsBuild > 0 {
		status.Blockers = append(status.Blockers,
			fmt.Sprintf("Windows build %d is too old for WSL2 (requires build 19041+)", status.WindowsBuild))
		status.Recommendations = append(status.Recommendations,
			"Update Windows to version 2004 (build 19041) or later")
	}

	// Step 2: Check if WSL feature is enabled via Registry
	status.WSLAvailable = detectWSLFeatureFromRegistry()

	if !status.WSLAvailable {
		// Try command-based detection as fallback
		status.WSLAvailable = detectWSLFeatureFromCommand(ctx)
	}

	if !status.WSLAvailable {
		status.Blockers = append(status.Blockers, "WSL feature is not enabled")
		status.Recommendations = append(status.Recommendations,
			"Run: wsl --install (from an Administrator PowerShell)")
		status.Recommendations = append(status.Recommendations,
			"Or: Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Windows-Subsystem-Linux")
	}

	// Step 3: If WSL is available, get detailed status
	if status.WSLAvailable {
		detectWSLDetails(ctx, status)
	}

	// Step 4: Check Hyper-V (required for WSL2)
	status.HyperVAvailable = detectHyperV(ctx)
	if !status.HyperVAvailable && status.WSLAvailable {
		// WSL1 works without Hyper-V, but WSL2 requires it
		status.Recommendations = append(status.Recommendations,
			"For WSL2: Enable Hyper-V — dism.exe /online /enable-feature /featurename:Microsoft-Hyper-V-All /all /norestart")
	}

	// Step 5: Determine overall readiness
	status.Ready = status.WSLAvailable &&
		status.WSLVersion == "2" &&
		(status.WindowsBuild >= 19041 || status.WindowsBuild == 0) &&
		status.HyperVAvailable

	if !status.Ready && len(status.Blockers) == 0 {
		if status.WSLVersion == "1" {
			status.Blockers = append(status.Blockers, "WSL1 is installed but WSL2 is required")
			status.Recommendations = append(status.Recommendations,
				"Run: wsl --set-default-version 2")
			status.Recommendations = append(status.Recommendations,
				"Run: wsl --update to get the latest WSL2 kernel")
		}
	}

	return status
}

// detectWSLFeatureFromRegistry checks if the WSL feature is enabled
// by looking for the Lxss registry key. This key is created when
// WSL is installed on Windows.
func detectWSLFeatureFromRegistry() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Lxss`,
		registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	key.Close()
	return true
}

// detectWSLFeatureFromCommand checks if WSL is available by
// attempting to run `wsl --status`. This is a fallback for when
// Registry access is not available.
//
// Per Zero-Trust: routes through bridgeExecFn (SanitizeAndExecute),
// NOT through raw exec.Command.
func detectWSLFeatureFromCommand(ctx context.Context) bool {
	_, err := bridgeExecFn(ctx, "wsl", "--status")
	return err == nil
}

// detectWSLDetails populates the WSL2Status with detailed information
// about installed distributions, WSL version, and kernel.
//
// Per Zero-Trust: all command execution routes through bridgeExecFn.
func detectWSLDetails(ctx context.Context, status *WSL2Status) {
	// Get WSL version info
	output, err := bridgeExecFn(ctx, "wsl", "--version")
	if err == nil {
		parseWSLVersionOutput(output, status)
	}

	// Get default WSL version
	output, err = bridgeExecFn(ctx, "wsl", "--status")
	if err == nil {
		parseWSLStatusOutput(output, status)
	}

	// List installed distributions
	output, err = bridgeExecFn(ctx, "wsl", "--list", "--verbose")
	if err == nil {
		status.Distros = parseWSLDistroList(output)
		if len(status.Distros) > 0 {
			for _, d := range status.Distros {
				if d.Default {
					status.DefaultDistro = d.Name
					break
				}
			}
		}
	}
}

// parseWSLVersionOutput parses `wsl --version` output.
// Example output:
//
//	WSL version: 2.0.9.0
//	Kernel version: 5.15.133.1-1
//	Windows version: 10.0.22631.3007
func parseWSLVersionOutput(output string, status *WSL2Status) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "WSL version:") {
			status.WSLVersion = strings.TrimSpace(strings.TrimPrefix(line, "WSL version:"))
			// Extract major version (e.g., "2.0.9.0" → the "2" is WSL2)
			if strings.HasPrefix(status.WSLVersion, "2.") {
				status.WSLVersion = "2"
			} else if strings.HasPrefix(status.WSLVersion, "1.") {
				status.WSLVersion = "1"
			}
		}
		if strings.HasPrefix(line, "Kernel version:") {
			status.KernelVersion = strings.TrimSpace(strings.TrimPrefix(line, "Kernel version:"))
		}
	}
}

// parseWSLStatusOutput parses `wsl --status` output.
// Example output:
//
//	Default Version: 2
//	...
func parseWSLStatusOutput(output string, status *WSL2Status) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Default Version:") {
			v := strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
			if v == "2" {
				status.WSLVersion = "2"
			} else if v == "1" {
				status.WSLVersion = "1"
			}
		}
	}
}

// parseWSLDistroList parses `wsl --list --verbose` output.
// Example output:
//
//	  NAME              STATE           VERSION
//	* Ubuntu            Running         2
//	  Debian            Stopped         2
func parseWSLDistroList(output string) []WSLDistro {
	var distros []WSLDistro

	lines := strings.Split(output, "\n")
	if len(lines) < 2 {
		return distros
	}

	// Skip header line
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Default distribution is marked with *
		isDefault := false
		if strings.HasPrefix(line, "*") {
			isDefault = true
			line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
		}

		fields := strings.Fields(line)
		if len(fields) >= 3 {
			distros = append(distros, WSLDistro{
				Name:    fields[0],
				State:   fields[1],
				Version: fields[2],
				Default: isDefault,
			})
		} else if len(fields) >= 1 {
			// Some versions of WSL have different output format
			distros = append(distros, WSLDistro{
				Name:    fields[0],
				Default: isDefault,
			})
		}
	}

	return distros
}

// detectHyperV checks if Hyper-V is available on this system.
// Hyper-V is required for WSL2.
//
// Uses Registry first (no command execution), then falls back to
// dism.exe via bridgeExecFn (Zero-Trust compliant).
func detectHyperV(ctx context.Context) bool {
	// Method 1: Check Registry for Hyper-V feature
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion\Virtualization`,
		registry.QUERY_VALUE)
	if err == nil {
		key.Close()
		return true
	}

	// Method 2: Try checking via DISM (Deployment Image Servicing and Management)
	// Per Zero-Trust: routes through bridgeExecFn (SanitizeAndExecute)
	output, err := bridgeExecFn(ctx, "dism", "/online", "/get-featureinfo", "/featurename:Microsoft-Hyper-V")
	if err == nil {
		return strings.Contains(output, "State : Enabled") ||
			strings.Contains(output, "State : Enable Pending")
	}

	return false
}

// detectWindowsBuild is a helper to get just the build number.
func detectWindowsBuild() int {
	_, build := detectWindowsVersion()
	return build
}

// detectWindowsProductName is a helper to get just the product name.
func detectWindowsProductName() string {
	version, _ := detectWindowsVersion()
	return version
}

// detectWindowsPackageManager detects available Windows package managers.
// Unlike Linux, Windows doesn't have a single dominant PM.
// We check for: winget (official), scoop, choco (community).
//
// Per Zero-Trust: routes through bridgeExecFn (SanitizeAndExecute),
// NOT through raw exec.Command.
func detectWindowsPackageManager(ctx context.Context) string {
	// Check for package managers in order of preference
	managers := []struct {
		bin  string
		name string
	}{
		{"winget", "winget"},  // Official Windows Package Manager
		{"scoop", "scoop"},    // Community, portable
		{"choco", "choco"},    // Community, PowerShell-based
	}

	for _, pm := range managers {
		// Per Zero-Trust: use bridgeExecFn instead of raw exec.Command
		_, err := bridgeExecFn(ctx, "where", pm.bin)
		if err == nil {
			return pm.name
		}
	}

	return "windows" // Generic fallback — no PM detected
}

// DetectWSL2Status is the main entry point for V4's WSL2 detection.
// It returns a comprehensive WSL2Status struct with all detected information.
//
// Per V4: "A Windows .exe version of your Go tool that checks if WSL2 is enabled."
// This is the "Spy" function — it reconnoiters the Windows WSL2 terrain.
func DetectWSL2Status(ctx context.Context) *WSL2Status {
	return detectWSLAvailability(ctx)
}

// IsWSLCommandAvailable checks if this command should be available
// on the current platform. On Windows, it always returns true.
// On Linux, it returns false (use `nexus probe` instead).
func IsWSLCommandAvailable() bool {
	return true // This file is only compiled on Windows
}
