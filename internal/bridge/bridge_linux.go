//go:build linux

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
	"os"
	"runtime"
	"strings"
)

// detectEnvironmentImpl probes the Linux environment.
// This file is compiled ONLY on Linux (//go:build linux).
//
// It detects:
// - Whether we're running inside WSL2 (via /proc/version)
// - The Linux distribution (via /etc/os-release)
// - The available package manager (via /usr/bin/ and /sbin/ checks)
//
// Note: WSL2Status is nil on Linux — on Windows, that field is populated
// by the host-side Spy detection. On Linux, we detect WSL2 from inside
// the guest via /proc/version, and that information is in IsWSL2.
func detectEnvironmentImpl(ctx context.Context) *EnvironmentInfo {
	env := &EnvironmentInfo{
		IsWindows: false,
		// WSL2Status is nil on Linux — the Spy runs on Windows only
	}

	env.IsWSL2 = detectWSL2()
	env.IsNativeLinux = !env.IsWSL2
	env.Distro = detectDistro()
	env.PackageManager = detectPackageManager()

	_ = runtime.GOOS // Already guaranteed to be "linux" by build tag

	return env
}

// detectWSL2 checks if we're running inside Windows Subsystem for Linux.
// WSL2 places a marker in /proc/version containing "microsoft" or "wsl".
func detectWSL2() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	content := strings.ToLower(string(data))
	return strings.Contains(content, "microsoft") || strings.Contains(content, "wsl")
}

// detectDistro reads /etc/os-release to identify the Linux distribution.
func detectDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}

	var name, version string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "NAME=") {
			name = strings.Trim(strings.TrimPrefix(line, "NAME="), "\"")
		}
		if strings.HasPrefix(line, "VERSION=") {
			version = strings.Trim(strings.TrimPrefix(line, "VERSION="), "\"")
		}
	}

	if name != "" && version != "" {
		return name + " " + version
	}
	if name != "" {
		return name
	}
	return "unknown"
}

// detectPackageManager determines the appropriate package manager
// based on the detected distribution.
// Per the plan: "It translates the package lists into the appropriate
// native commands (apt-get install, pacman -S, etc.)"
func detectPackageManager() string {
	// Check for package manager binaries in order of preference
	managers := []struct {
		bin  string
		name string
	}{
		{"apt-get", "apt"},
		{"pacman", "pacman"},
		{"dnf", "dnf"},
		{"yum", "yum"},
		{"apk", "apk"},
	}

	for _, pm := range managers {
		if _, err := os.Stat("/usr/bin/" + pm.bin); err == nil {
			return pm.name
		}
		// Also check /sbin for apk
		if _, err := os.Stat("/sbin/" + pm.bin); err == nil {
			return pm.name
		}
	}

	return "unknown"
}
