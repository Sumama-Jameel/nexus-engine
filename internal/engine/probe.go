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
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// SystemInfo holds all probed system information collected by the Probe function.
// It provides a comprehensive snapshot of the host's operating system, hardware,
// network, and virtualization state, which drives all downstream decisions about
// package managers, profiles, and platform-specific behavior.
type SystemInfo struct {
	// OS is the operating system identifier as reported by runtime.GOOS
	// (e.g., "linux", "windows", "darwin").
	OS string `json:"os"`
	// Arch is the CPU architecture as reported by runtime.GOARCH
	// (e.g., "amd64", "arm64").
	Arch string `json:"arch"`
	// Kernel is the kernel release string from uname -r (Linux only).
	Kernel string `json:"kernel"`
	// Hostname is the machine's network hostname.
	Hostname string `json:"hostname"`
	// CPUModel is the processor model name, read from /proc/cpuinfo on Linux.
	CPUModel string `json:"cpu_model"`
	// CPUCores is the number of logical CPU cores available.
	CPUCores int `json:"cpu_cores"`
	// RAMTotalMB is the total installed RAM in megabytes.
	RAMTotalMB int `json:"ram_total_mb"`
	// RAMAvailableMB is the currently available RAM in megabytes.
	RAMAvailableMB int `json:"ram_available_mb"`
	// DiskTotalGB is the total disk capacity of the root partition in gigabytes.
	DiskTotalGB float64 `json:"disk_total_gb"`
	// DiskUsedGB is the used disk space on the root partition in gigabytes.
	DiskUsedGB float64 `json:"disk_used_gb"`
	// GPU is the GPU model name detected via lspci, or "N/A" if unavailable.
	GPU string `json:"gpu"`
	// IsWSL2 indicates whether the system is running inside Windows Subsystem
	// for Linux version 2, detected by checking /proc/version for Microsoft markers.
	IsWSL2 bool `json:"is_wsl2"`
	// NetworkIP is the primary network IP address from hostname -I.
	NetworkIP string `json:"network_ip"`
	// Uptime is the system uptime as a human-readable duration string.
	Uptime string `json:"uptime"`
	// DistroID is the Linux distribution identifier from /etc/os-release ID field
	// (e.g., "ubuntu", "kali", "pop", "fedora", "arch"). Empty on non-Linux.
	DistroID string `json:"distro_id"`
	// DistroVersion is the version string from /etc/os-release VERSION_ID field
	// (e.g., "24.04", "2024.1", "39"). Empty on non-Linux or if undetectable.
	DistroVersion string `json:"distro_version"`
	// ProbedAt is the UTC timestamp when the probe was executed.
	ProbedAt time.Time `json:"probed_at"`
}

// Probe executes the system probe: queries OS, hardware, and network information.
// Per the Nexus Protocol: "If the Brain doesn't know where it is, it can't control anything."
func Probe(ctx context.Context) (*SystemInfo, error) {
	info := &SystemInfo{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		ProbedAt: time.Now().UTC(),
	}

	// Run all probes concurrently using goroutines, respecting context cancellation.
	errCh := make(chan error, 13)

	go func() { errCh <- probeKernel(ctx, info) }()
	go func() { errCh <- probeHostname(info) }()
	go func() { errCh <- probeCPU(info) }()
	go func() { errCh <- probeRAM(ctx, info) }()
	go func() { errCh <- probeDisk(info) }()
	go func() { errCh <- probeGPU(ctx, info) }()
	go func() { errCh <- probeWSL2(info) }()
	go func() { errCh <- probeNetwork(ctx, info) }()
	go func() { errCh <- probeUptime(ctx, info) }()
	go func() { errCh <- probeDistro(info) }()

	// Collect errors (non-fatal: we return whatever we could probe)
	var errs []error
	for i := 0; i < 10; i++ {
		if err := <-errCh; err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return info, fmt.Errorf("probe completed with %d warnings", len(errs))
	}
	return info, nil
}

// probeExecFn is the execution function used by probe functions.
// It defaults to SanitizeAndExecute but can be overridden in tests
// to inject mock command output. This is a package-level variable
// intentionally: the probe functions are called concurrently within
// Probe(), and this avoids threading the exec function through every
// probe sub-function. It is set once in TestMain before any parallel
// tests run, so there is no data race in practice.
var probeExecFn = SanitizeAndExecute

// --- Individual Probes ---

func probeKernel(ctx context.Context, info *SystemInfo) error {
	out, err := SanitizeAndExecute(ctx, "uname", "-r")
	if err != nil {
		return err
	}
	info.Kernel = strings.TrimSpace(out)
	return nil
}

func probeHostname(info *SystemInfo) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	info.Hostname = hostname
	return nil
}

func probeCPU(info *SystemInfo) error {
	// Core count
	info.CPUCores = runtime.NumCPU()

	// CPU model — read from /proc/cpuinfo on Linux
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/cpuinfo")
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "model name") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						info.CPUModel = strings.TrimSpace(parts[1])
					}
					break
				}
			}
		}
	}

	if info.CPUModel == "" {
		info.CPUModel = "Unknown"
	}
	return nil
}

func probeRAM(ctx context.Context, info *SystemInfo) error {
	// Use free command on Linux
	if runtime.GOOS == "linux" {
		out, err := SanitizeAndExecute(ctx, "free", "-m")
		if err != nil {
			return err
		}
		lines := strings.Split(out, "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 2 {
				info.RAMTotalMB, _ = strconv.Atoi(fields[1])
			}
			if len(fields) >= 4 {
				info.RAMAvailableMB, _ = strconv.Atoi(fields[3])
			}
		}
	}
	return nil
}

// probeDisk is implemented in platform-specific files:
// - probe_disk_linux.go  (uses syscall.Statfs)
// - probe_disk_windows.go (uses GetDiskFreeSpaceEx)

func probeGPU(ctx context.Context, info *SystemInfo) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	out, err := probeExecFn(ctx, "lspci")
	if err != nil {
		// lspci may not be available — non-fatal
		info.GPU = "N/A (lspci not available)"
		return nil
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(line), "vga") || strings.Contains(strings.ToLower(line), "3d") || strings.Contains(strings.ToLower(line), "display") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				info.GPU = strings.TrimSpace(parts[1])
				return nil
			}
		}
	}
	info.GPU = "N/A"
	return nil
}

func probeWSL2(info *SystemInfo) error {
	if runtime.GOOS != "linux" {
		info.IsWSL2 = false
		return nil
	}
	// WSL2 places a marker file at /proc/version containing "microsoft" or "WSL"
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		info.IsWSL2 = false
		return nil
	}
	content := strings.ToLower(string(data))
	info.IsWSL2 = strings.Contains(content, "microsoft") || strings.Contains(content, "wsl")
	return nil
}

func probeNetwork(ctx context.Context, info *SystemInfo) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	// Use hostname -I to get all IP addresses
	out, err := SanitizeAndExecute(ctx, "hostname", "-I")
	if err != nil {
		return err
	}
	ips := strings.Fields(out)
	if len(ips) > 0 {
		info.NetworkIP = ips[0]
	} else {
		info.NetworkIP = "N/A"
	}
	return nil
}

func probeDistro(info *SystemInfo) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		// Not all systems have /etc/os-release — non-fatal
		return nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ID=") {
			val := strings.TrimPrefix(line, "ID=")
			val = strings.Trim(val, `"'`)
			info.DistroID = val
		} else if strings.HasPrefix(line, "VERSION_ID=") {
			val := strings.TrimPrefix(line, "VERSION_ID=")
			val = strings.Trim(val, `"'`)
			info.DistroVersion = val
		}
		if info.DistroID != "" && info.DistroVersion != "" {
			break
		}
	}
	return nil
}

func probeUptime(ctx context.Context, info *SystemInfo) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	out, err := SanitizeAndExecute(ctx, "uptime", "-p")
	if err != nil {
		return err
	}
	info.Uptime = strings.TrimSpace(strings.TrimPrefix(out, "up "))
	return nil
}

// FormatSystemInfo returns a human-readable, terminal-formatted summary of the
// system probe results. The output includes branded headers, Unicode box-drawing
// characters, and emoji indicators for each hardware subsystem.
func FormatSystemInfo(info *SystemInfo) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString("  ╔══════════════════════════════════════════════════╗\n")
	sb.WriteString("  ║           NEXUS PROTOCOL — SYSTEM PROBE          ║\n")
	sb.WriteString("  ╚══════════════════════════════════════════════════╝\n")
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("  🖥️  OS:            %s/%s\n", info.OS, info.Arch))
	sb.WriteString(fmt.Sprintf("  🧬 Kernel:        %s\n", info.Kernel))
	sb.WriteString(fmt.Sprintf("  🏠 Hostname:      %s\n", info.Hostname))
	sb.WriteString("\n")

	sb.WriteString("  ── CPU ─────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  ⚡ Model:         %s\n", info.CPUModel))
	sb.WriteString(fmt.Sprintf("  🔢 Cores:         %d\n", info.CPUCores))
	sb.WriteString("\n")

	sb.WriteString("  ── MEMORY ──────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  💾 Total RAM:     %d MB\n", info.RAMTotalMB))
	sb.WriteString(fmt.Sprintf("  ✅ Available:     %d MB\n", info.RAMAvailableMB))
	sb.WriteString("\n")

	sb.WriteString("  ── STORAGE ─────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  💿 Total Disk:    %.1f GB\n", info.DiskTotalGB))
	sb.WriteString(fmt.Sprintf("  📊 Used:          %.1f GB\n", info.DiskUsedGB))
	sb.WriteString(fmt.Sprintf("  🆓 Free:          %.1f GB\n", info.DiskTotalGB-info.DiskUsedGB))
	sb.WriteString("\n")

	sb.WriteString("  ── HARDWARE ────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  🎮 GPU:           %s\n", info.GPU))
	sb.WriteString(fmt.Sprintf("  🌐 Network IP:    %s\n", info.NetworkIP))
	sb.WriteString(fmt.Sprintf("  ⏱️  Uptime:        %s\n", info.Uptime))
	sb.WriteString("\n")

	sb.WriteString("  ── ENVIRONMENT ─────────────────────────────────\n")
	if info.IsWSL2 {
		sb.WriteString("  🪟 WSL2:          DETECTED (Windows Subsystem for Linux)\n")
	} else {
		sb.WriteString("  🐧 WSL2:          Not detected (Native Linux)\n")
	}
	sb.WriteString("\n")

	sb.WriteString("  ══════════════════════════════════════════════════\n")
	sb.WriteString(fmt.Sprintf("  Probed at: %s\n", info.ProbedAt.Format(time.RFC3339)))
	sb.WriteString("  ══════════════════════════════════════════════════\n")

	return sb.String()
}
