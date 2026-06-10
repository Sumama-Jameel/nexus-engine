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
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// VerifyInstallation tests
// ---------------------------------------------------------------------------

// TestVerifyInstallation_AllVerified verifies that when both IsInstalled
// and the binary check succeed, all packages are marked as verified.
func TestVerifyInstallation_AllVerified(t *testing.T) {
	pm := newMockPackageManager("apt")
	pm.installed["git"] = true
	pm.installed["curl"] = true

	execFn := simpleMockExec()
	ctx := context.Background()

	results := VerifyInstallation(ctx, pm, []string{"git", "curl"}, execFn)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Verified {
			t.Errorf("package %q should be verified", r.Package)
		}
		if r.Status != "verified" {
			t.Errorf("package %q status = %q, want %q", r.Package, r.Status, "verified")
		}
	}
}

// TestVerifyInstallation_NotInstalled verifies that when IsInstalled
// returns false, the package is marked as not_found.
func TestVerifyInstallation_NotInstalled(t *testing.T) {
	pm := newMockPackageManager("apt")
	// git is NOT in pm.installed, so IsInstalled returns false

	execFn := simpleMockExec()
	ctx := context.Background()

	results := VerifyInstallation(ctx, pm, []string{"git"}, execFn)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Verified {
		t.Error("package should NOT be verified")
	}
	if results[0].Status != "not_found" {
		t.Errorf("status = %q, want %q", results[0].Status, "not_found")
	}
}

// TestVerifyInstallation_Broken verifies that when IsInstalled returns
// true but the binary check fails, the package is marked as broken.
func TestVerifyInstallation_Broken(t *testing.T) {
	pm := newMockPackageManager("apt")
	pm.installed["git"] = true

	execFn := failingMockExec()
	ctx := context.Background()

	results := VerifyInstallation(ctx, pm, []string{"git"}, execFn)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Verified {
		t.Error("package should NOT be verified when binary check fails")
	}
	if results[0].Status != "broken" {
		t.Errorf("status = %q, want %q", results[0].Status, "broken")
	}
}

// TestVerifyInstallation_UnknownPackage verifies that packages not in
// the verifiable map are trusted by default (verified = true).
func TestVerifyInstallation_UnknownPackage(t *testing.T) {
	pm := newMockPackageManager("apt")
	pm.installed["custom-package"] = true

	execFn := failingMockExec() // even a failing exec shouldn't matter
	ctx := context.Background()

	results := VerifyInstallation(ctx, pm, []string{"custom-package"}, execFn)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Verified {
		t.Error("unknown packages should be trusted by default")
	}
	if results[0].Status != "verified" {
		t.Errorf("status = %q, want %q", results[0].Status, "verified")
	}
}

// TestVerifyInstallation_JavaSpecialCase verifies that java with
// stderr-only output is still considered functional.
func TestVerifyInstallation_JavaSpecialCase(t *testing.T) {
	pm := newMockPackageManager("apt")
	pm.installed["java"] = true

	// Simulate java -version output that goes to stderr
	javaExecFn := func(ctx context.Context, command string, args ...string) (string, error) {
		return "", fmt.Errorf("EXEC: java -version (stderr: openjdk version \"21.0.1\" 2023-10-17)")
	}
	ctx := context.Background()

	results := VerifyInstallation(ctx, pm, []string{"java"}, javaExecFn)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Verified {
		t.Error("java with stderr-only output should still be verified")
	}
	if results[0].Status != "verified" {
		t.Errorf("status = %q, want %q", results[0].Status, "verified")
	}
}

// TestVerifyInstallation_JavaNotFound verifies that java with a
// "not found" error is NOT verified (not a stderr-only case).
func TestVerifyInstallation_JavaNotFound(t *testing.T) {
	pm := newMockPackageManager("apt")
	pm.installed["java"] = true

	javaNotFoundFn := func(ctx context.Context, command string, args ...string) (string, error) {
		return "", fmt.Errorf("EXEC: command 'java' failed: executable file not found")
	}
	ctx := context.Background()

	results := VerifyInstallation(ctx, pm, []string{"java"}, javaNotFoundFn)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Verified {
		t.Error("java with 'not found' error should NOT be verified")
	}
	if results[0].Status != "broken" {
		t.Errorf("status = %q, want %q", results[0].Status, "broken")
	}
}

// TestVerifyInstallation_MultiplePackages verifies mixed outcomes.
func TestVerifyInstallation_MultiplePackages(t *testing.T) {
	pm := newMockPackageManager("apt")
	pm.installed["git"] = true
	pm.installed["curl"] = true
	// "missing" is NOT installed
	pm.installed["custom-pkg"] = true

	execFn := simpleMockExec()
	ctx := context.Background()

	results := VerifyInstallation(ctx, pm, []string{"git", "curl", "missing", "custom-pkg"}, execFn)

	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}

	// git: installed + binary check → verified
	// curl: installed + binary check → verified
	// missing: not installed → not_found
	// custom-pkg: installed + not in verifiable map → trusted (verified)
	expected := map[string]struct {
		status   string
		verified bool
	}{
		"git":         {true, "verified"},
		"curl":        {true, "verified"},
		"missing":     {false, "not_found"},
		"custom-pkg":  {true, "verified"},
	}

	for _, r := range results {
		exp, ok := expected[r.Package]
		if !ok {
			t.Errorf("unexpected package %q in results", r.Package)
			continue
		}
		if r.Verified != exp.verified {
			t.Errorf("package %q: Verified = %v, want %v", r.Package, r.Verified, exp.verified)
		}
		if r.Status != exp.status {
			t.Errorf("package %q: Status = %q, want %q", r.Package, r.Status, exp.status)
		}
	}
}

// ---------------------------------------------------------------------------
// verifyBinary tests
// ---------------------------------------------------------------------------

func TestVerifyBinary_KnownPackage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pkg      string
		execOK   bool
		expected bool
	}{
		{"git", true, true},
		{"curl", true, true},
		{"wget", true, true},
		{"vim", true, true},
		{"python3", true, true},
		{"nodejs", true, true},
		{"npm", true, true},
		{"zsh", true, true},
		{"htop", true, true},
		{"tmux", true, true},
		{"chezmoi", true, true},
		{"git", false, false},   // exec fails → not verified
		{"curl", false, false},  // exec fails → not verified
	}

	for _, tt := range tests {
		tt := tt
		name := fmt.Sprintf("%s/execOK_%v", tt.pkg, tt.execOK)
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var execFn ExecFunc
			if tt.execOK {
				execFn = simpleMockExec()
			} else {
				execFn = failingMockExec()
			}

			got := verifyBinary(context.Background(), execFn, tt.pkg)
			if got != tt.expected {
				t.Errorf("verifyBinary(%q) = %v, want %v", tt.pkg, got, tt.expected)
			}
		})
	}
}

func TestVerifyBinary_UnknownPackage(t *testing.T) {
	t.Parallel()

	// Unknown packages should be trusted (return true) regardless of execFn
	result := verifyBinary(context.Background(), failingMockExec(), "some-unknown-package")
	if !result {
		t.Error("unknown packages should be trusted by default")
	}
}

func TestVerifyBinary_EmptyPackage(t *testing.T) {
	t.Parallel()

	// Empty string is not in the verifiable map, so it's trusted
	result := verifyBinary(context.Background(), failingMockExec(), "")
	if !result {
		t.Error("empty package name should be trusted by default")
	}
}

// ---------------------------------------------------------------------------
// FormatVerifyResults tests
// ---------------------------------------------------------------------------

func TestFormatVerifyResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		results      []VerifyResult
		wantContains []string
	}{
		{
			name: "all_verified",
			results: []VerifyResult{
				{Package: "git", Verified: true, Status: "verified"},
				{Package: "curl", Verified: true, Status: "verified"},
			},
			wantContains: []string{"Verified: 2", "Broken: 0", "Not Found: 0"},
		},
		{
			name: "mixed_results",
			results: []VerifyResult{
				{Package: "git", Verified: true, Status: "verified"},
				{Package: "broken-pkg", Verified: false, Status: "broken"},
				{Package: "missing-pkg", Verified: false, Status: "not_found"},
			},
			wantContains: []string{"Verified: 1", "Broken: 1", "Not Found: 1"},
		},
		{
			name:         "empty",
			results:      []VerifyResult{},
			wantContains: []string{"Verified: 0", "Broken: 0", "Not Found: 0"},
		},
		{
			name: "all_broken",
			results: []VerifyResult{
				{Package: "a", Verified: false, Status: "broken"},
				{Package: "b", Verified: false, Status: "broken"},
			},
			wantContains: []string{"Verified: 0", "Broken: 2"},
		},
		{
			name: "all_not_found",
			results: []VerifyResult{
				{Package: "x", Verified: false, Status: "not_found"},
			},
			wantContains: []string{"Not Found: 1"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			output := FormatVerifyResults(tt.results)
			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q.\nGot: %s", want, output)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// VerifyResult struct tests
// ---------------------------------------------------------------------------

func TestVerifyResult_Fields(t *testing.T) {
	t.Parallel()

	vr := VerifyResult{
		Package:  "git",
		Verified: true,
		Status:   "verified",
	}

	if vr.Package != "git" {
		t.Errorf("Package = %q, want %q", vr.Package, "git")
	}
	if !vr.Verified {
		t.Error("Verified should be true")
	}
	if vr.Status != "verified" {
		t.Errorf("Status = %q, want %q", vr.Status, "verified")
	}
}

// TestVerifyInstallation_EmptyPackages verifies behavior with no packages.
func TestVerifyInstallation_EmptyPackages(t *testing.T) {
	pm := newMockPackageManager("apt")
	execFn := simpleMockExec()
	ctx := context.Background()

	results := VerifyInstallation(ctx, pm, []string{}, execFn)

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(results))
	}
}
