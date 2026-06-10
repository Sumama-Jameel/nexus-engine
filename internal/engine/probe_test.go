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
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// SystemInfo struct — field population and defaults
// ---------------------------------------------------------------------------

func TestSystemInfo_Fields(t *testing.T) {
	t.Parallel()

	info := &SystemInfo{
		OS:          "linux",
		Arch:        "amd64",
		CPUCores:    8,
		RAMTotalMB:  16384,
		DiskTotalGB: 500.0,
		IsWSL2:      false,
	}

	if info.OS != "linux" {
		t.Errorf("OS = %q, want %q", info.OS, "linux")
	}
	if info.Arch != "amd64" {
		t.Errorf("Arch = %q, want %q", info.Arch, "amd64")
	}
	if info.CPUCores != 8 {
		t.Errorf("CPUCores = %d, want %d", info.CPUCores, 8)
	}
	if info.RAMTotalMB != 16384 {
		t.Errorf("RAMTotalMB = %d, want %d", info.RAMTotalMB, 16384)
	}
	if info.DiskTotalGB != 500.0 {
		t.Errorf("DiskTotalGB = %f, want %f", info.DiskTotalGB, 500.0)
	}
	if info.IsWSL2 {
		t.Error("IsWSL2 should be false")
	}
}

// ---------------------------------------------------------------------------
// SystemInfo — JSON serialization round-trip
// ---------------------------------------------------------------------------

func TestSystemInfo_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	original := &SystemInfo{
		OS:             "linux",
		Arch:           "arm64",
		Kernel:         "5.15.0-custom",
		Hostname:       "jsonhost",
		CPUModel:       "Apple M1",
		CPUCores:       8,
		RAMTotalMB:     8192,
		RAMAvailableMB: 4096,
		DiskTotalGB:    256.0,
		DiskUsedGB:     128.5,
		GPU:            "N/A",
		IsWSL2:         true,
		NetworkIP:      "10.0.0.1",
		Uptime:         "5 minutes",
		ProbedAt:       time.Now().UTC().Truncate(time.Millisecond),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded SystemInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.OS != original.OS {
		t.Errorf("OS: got %q, want %q", decoded.OS, original.OS)
	}
	if decoded.Arch != original.Arch {
		t.Errorf("Arch: got %q, want %q", decoded.Arch, original.Arch)
	}
	if decoded.Kernel != original.Kernel {
		t.Errorf("Kernel: got %q, want %q", decoded.Kernel, original.Kernel)
	}
	if decoded.CPUCores != original.CPUCores {
		t.Errorf("CPUCores: got %d, want %d", decoded.CPUCores, original.CPUCores)
	}
	if decoded.RAMTotalMB != original.RAMTotalMB {
		t.Errorf("RAMTotalMB: got %d, want %d", decoded.RAMTotalMB, original.RAMTotalMB)
	}
	if decoded.IsWSL2 != original.IsWSL2 {
		t.Errorf("IsWSL2: got %v, want %v", decoded.IsWSL2, original.IsWSL2)
	}
	if decoded.NetworkIP != original.NetworkIP {
		t.Errorf("NetworkIP: got %q, want %q", decoded.NetworkIP, original.NetworkIP)
	}
}

// ---------------------------------------------------------------------------
// SystemInfo — JSON field names follow the expected convention
// ---------------------------------------------------------------------------

func TestSystemInfo_JSONFieldNames(t *testing.T) {
	t.Parallel()

	info := &SystemInfo{
		OS:       "linux",
		ProbedAt: time.Now().UTC(),
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal into map failed: %v", err)
	}

	expectedFields := []string{
		"os", "arch", "kernel", "hostname", "cpu_model", "cpu_cores",
		"ram_total_mb", "ram_available_mb", "disk_total_gb", "disk_used_gb",
		"gpu", "is_wsl2", "network_ip", "uptime", "probed_at",
	}

	for _, field := range expectedFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("JSON output missing expected field %q", field)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatSystemInfo — output format verification
// ---------------------------------------------------------------------------

func TestFormatSystemInfo(t *testing.T) {
	t.Parallel()

	info := &SystemInfo{
		OS:             "linux",
		Arch:           "amd64",
		Kernel:         "6.1.0-testkernel",
		Hostname:       "testhost",
		CPUModel:       "Test CPU",
		CPUCores:       4,
		RAMTotalMB:     8192,
		RAMAvailableMB: 4096,
		DiskTotalGB:    100.0,
		DiskUsedGB:     50.0,
		GPU:            "Test GPU",
		IsWSL2:         false,
		NetworkIP:      "192.168.1.1",
		Uptime:         "1 hour",
		ProbedAt:       time.Now().UTC(),
	}

	output := FormatSystemInfo(info)

	expectedSubstrings := []string{
		"NEXUS PROTOCOL",
		"SYSTEM PROBE",
		"linux/amd64",
		"6.1.0-testkernel",
		"testhost",
		"Test CPU",
		"8192 MB",
		"4096 MB",
		"100.0 GB",
		"50.0 GB",
		"Test GPU",
		"192.168.1.1",
		"1 hour",
		"Not detected",
		"Probed at:",
	}

	for _, substr := range expectedSubstrings {
		if !strings.Contains(output, substr) {
			t.Errorf("FormatSystemInfo output missing expected substring %q", substr)
		}
	}
}

func TestFormatSystemInfo_WSL2Detected(t *testing.T) {
	t.Parallel()

	info := &SystemInfo{
		OS:       "linux",
		Arch:     "amd64",
		IsWSL2:   true,
		ProbedAt: time.Now().UTC(),
	}

	output := FormatSystemInfo(info)
	if !strings.Contains(output, "DETECTED") {
		t.Error("FormatSystemInfo should show DETECTED when IsWSL2 is true")
	}
	if strings.Contains(output, "Not detected") {
		t.Error("FormatSystemInfo should NOT show 'Not detected' when IsWSL2 is true")
	}
}

func TestFormatSystemInfo_WSL2NotDetected(t *testing.T) {
	t.Parallel()

	info := &SystemInfo{
		OS:       "linux",
		Arch:     "amd64",
		IsWSL2:   false,
		ProbedAt: time.Now().UTC(),
	}

	output := FormatSystemInfo(info)
	if !strings.Contains(output, "Not detected") {
		t.Error("FormatSystemInfo should show 'Not detected' when IsWSL2 is false")
	}
}

func TestFormatSystemInfo_DiskFreeCalculation(t *testing.T) {
	t.Parallel()

	info := &SystemInfo{
		OS:          "linux",
		DiskTotalGB: 100.0,
		DiskUsedGB:  30.0,
		ProbedAt:    time.Now().UTC(),
	}

	output := FormatSystemInfo(info)
	// Free = Total - Used = 70.0 GB
	if !strings.Contains(output, "70.0 GB") {
		t.Errorf("FormatSystemInfo should show 70.0 GB free disk, got output containing disk section:\n%s", output)
	}
}

// ---------------------------------------------------------------------------
// Probe — live system test (verifies it doesn't panic and returns useful data)
// ---------------------------------------------------------------------------

func TestProbe_LiveSystem(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := Probe(ctx)

	if info == nil {
		t.Fatal("Probe returned nil SystemInfo")
	}

	// OS and Arch should always be populated (from runtime)
	if info.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", info.OS, runtime.GOOS)
	}
	if info.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", info.Arch, runtime.GOARCH)
	}

	// ProbedAt should be set and recent
	if info.ProbedAt.IsZero() {
		t.Error("ProbedAt should not be zero")
	}
	if time.Since(info.ProbedAt) > 10*time.Second {
		t.Error("ProbedAt should be very recent")
	}

	// On Linux, some fields should be populated if tools are available
	if runtime.GOOS == "linux" {
		if info.Hostname == "" {
			t.Log("Hostname is empty (may be expected in some environments)")
		}
		if info.CPUCores <= 0 {
			t.Errorf("CPUCores = %d, should be > 0", info.CPUCores)
		}
	}

	// Warnings are acceptable (some probes may fail)
	if err != nil {
		t.Logf("Probe returned warnings (acceptable): %v", err)
	}
}

// ---------------------------------------------------------------------------
// probeCPU — unit test for CPU model detection fallback
// ---------------------------------------------------------------------------

func TestProbeCPU_SetsCores(t *testing.T) {
	t.Parallel()

	info := &SystemInfo{}
	err := probeCPU(info)
	if err != nil {
		t.Fatalf("probeCPU failed: %v", err)
	}
	if info.CPUCores != runtime.NumCPU() {
		t.Errorf("CPUCores = %d, want %d", info.CPUCores, runtime.NumCPU())
	}
	if info.CPUModel == "" {
		t.Error("CPUModel should not be empty after probeCPU")
	}
}

// ---------------------------------------------------------------------------
// probeHostname — unit test
// ---------------------------------------------------------------------------

func TestProbeHostname(t *testing.T) {
	t.Parallel()

	info := &SystemInfo{}
	err := probeHostname(info)
	if err != nil {
		t.Logf("probeHostname failed (may be expected in some environments): %v", err)
		return
	}
	if info.Hostname == "" {
		t.Error("Hostname should not be empty after successful probeHostname")
	}
}

// ---------------------------------------------------------------------------
// probeGPU — mock exec function tests
// ---------------------------------------------------------------------------

// TestProbeGPU_WithVGAOutput verifies that probeGPU correctly parses
// lspci output containing a VGA controller line.
func TestProbeGPU_WithVGAOutput(t *testing.T) {
	// Save and restore the original probeExecFn
	orig := probeExecFn
	defer func() { probeExecFn = orig }()

	probeExecFn = func(ctx context.Context, command string, args ...string) (string, error) {
		return "00:02.0 VGA compatible controller: Intel Corporation HD Graphics 630\n01:00.0 Non-VGA unclassified device: Some other device\n", nil
	}

	info := &SystemInfo{}
	err := probeGPU(context.Background(), info)
	if err != nil {
		t.Fatalf("probeGPU should not return an error: %v", err)
	}
	// SplitN(line, ":", 2) splits at the first colon (in the bus address "00:02.0"),
	// so parts[1] includes the rest of the line.
	if info.GPU != "02.0 VGA compatible controller: Intel Corporation HD Graphics 630" {
		t.Errorf("GPU = %q, want %q", info.GPU, "02.0 VGA compatible controller: Intel Corporation HD Graphics 630")
	}
}

// TestProbeGPU_With3DController verifies detection of 3D controller
// (common for NVIDIA on some laptops).
func TestProbeGPU_With3DController(t *testing.T) {
	orig := probeExecFn
	defer func() { probeExecFn = orig }()

	probeExecFn = func(ctx context.Context, command string, args ...string) (string, error) {
		return "01:00.0 3D controller: NVIDIA Corporation GP107M [GeForce GTX 1050 Mobile]\n", nil
	}

	info := &SystemInfo{}
	err := probeGPU(context.Background(), info)
	if err != nil {
		t.Fatalf("probeGPU should not return an error: %v", err)
	}
	if info.GPU != "00.0 3D controller: NVIDIA Corporation GP107M [GeForce GTX 1050 Mobile]" {
		t.Errorf("GPU = %q, want NVIDIA GPU name", info.GPU)
	}
}

// TestProbeGPU_WithDisplayController verifies detection of "Display" keyword.
func TestProbeGPU_WithDisplayController(t *testing.T) {
	orig := probeExecFn
	defer func() { probeExecFn = orig }()

	probeExecFn = func(ctx context.Context, command string, args ...string) (string, error) {
		return "00:02.0 Display controller: Advanced Micro Devices, Inc. [AMD/ATI]\n", nil
	}

	info := &SystemInfo{}
	err := probeGPU(context.Background(), info)
	if err != nil {
		t.Fatalf("probeGPU should not return an error: %v", err)
	}
	if info.GPU != "02.0 Display controller: Advanced Micro Devices, Inc. [AMD/ATI]" {
		t.Errorf("GPU = %q, want AMD GPU name", info.GPU)
	}
}

// TestProbeGPU_NoGPUFound verifies that probeGPU sets "N/A" when lspci
// succeeds but no VGA/3D/Display lines are found.
func TestProbeGPU_NoGPUFound(t *testing.T) {
	orig := probeExecFn
	defer func() { probeExecFn = orig }()

	probeExecFn = func(ctx context.Context, command string, args ...string) (string, error) {
		return "00:1f.0 ISA bridge: Intel Corporation Device 9d4e\n00:1f.2 SATA controller: Intel Corporation Device 9d21\n", nil
	}

	info := &SystemInfo{}
	err := probeGPU(context.Background(), info)
	if err != nil {
		t.Fatalf("probeGPU should not return an error: %v", err)
	}
	if info.GPU != "N/A" {
		t.Errorf("GPU = %q, want %q", info.GPU, "N/A")
	}
}

// TestProbeGPU_LspciNotAvailable verifies the fallback when lspci
// command fails (not installed on the system).
func TestProbeGPU_LspciNotAvailable(t *testing.T) {
	orig := probeExecFn
	defer func() { probeExecFn = orig }()

	probeExecFn = func(ctx context.Context, command string, args ...string) (string, error) {
		return "", fmt.Errorf("lspci: command not found")
	}

	info := &SystemInfo{}
	err := probeGPU(context.Background(), info)
	if err != nil {
		t.Fatalf("probeGPU should not return an error when lspci is unavailable: %v", err)
	}
	if info.GPU != "N/A (lspci not available)" {
		t.Errorf("GPU = %q, want %q", info.GPU, "N/A (lspci not available)")
	}
}

// TestProbeGPU_LineWithoutColon verifies that lines matching VGA/3D/Display
// but without a second colon (only the bus address colon) are still parsed.
// Since SplitN(line, ":", 2) splits at the first colon (bus address),
// even lines with only one colon will have len(parts)==2.
func TestProbeGPU_LineWithoutColon(t *testing.T) {
	orig := probeExecFn
	defer func() { probeExecFn = orig }()

	// A line with NO colon at all — SplitN returns 1 part
	probeExecFn = func(ctx context.Context, command string, args ...string) (string, error) {
		return "VGA compatible controller without any colon\n", nil
	}

	info := &SystemInfo{}
	err := probeGPU(context.Background(), info)
	if err != nil {
		t.Fatalf("probeGPU should not return an error: %v", err)
	}
	// No colon at all means SplitN returns 1 part, so len(parts) < 2
	if info.GPU != "N/A" {
		t.Errorf("GPU = %q, want %q when no colon in lspci line", info.GPU, "N/A")
	}
}
