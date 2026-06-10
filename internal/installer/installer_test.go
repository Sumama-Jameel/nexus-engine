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
        "testing"

        "github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// ---------------------------------------------------------------------------
// Mock ExecFunc for testing — never executes real system commands
// ---------------------------------------------------------------------------

// mockExecFunc returns a controlled ExecFunc that responds to specific commands.
func mockExecFunc(responses map[string]mockResponse) ExecFunc {
        return func(ctx context.Context, command string, args ...string) (string, error) {
                // Build a key from command + args for lookup
                key := command
                for _, a := range args {
                        key += " " + a
                }
                if resp, ok := responses[key]; ok {
                        return resp.output, resp.err
                }
                // Default: command not found
                return "", fmt.Errorf("EXEC: command '%s' failed: executable file not found", command)
        }
}

type mockResponse struct {
        output string
        err    error
}

// simpleMockExec returns a minimal mock that always succeeds with empty output.
func simpleMockExec() ExecFunc {
        return func(ctx context.Context, command string, args ...string) (string, error) {
                return "mock output", nil
        }
}

// failingMockExec returns a mock that always fails.
func failingMockExec() ExecFunc {
        return func(ctx context.Context, command string, args ...string) (string, error) {
                return "", fmt.Errorf("mock execution failure")
        }
}

// ---------------------------------------------------------------------------
// NewInstaller — factory function tests
// ---------------------------------------------------------------------------

func TestNewInstaller_DebianFamily(t *testing.T) {
        t.Parallel()

        pm, err := NewInstaller("debian", simpleMockExec())
        if err != nil {
                t.Fatalf("NewInstaller(debian) failed: %v", err)
        }
        if pm.Name() != "apt" {
                t.Errorf("Name() = %q, want %q", pm.Name(), "apt")
        }
}

func TestNewInstaller_UbuntuFamily(t *testing.T) {
        t.Parallel()

        pm, err := NewInstaller("ubuntu", simpleMockExec())
        if err != nil {
                t.Fatalf("NewInstaller(ubuntu) failed: %v", err)
        }
        if pm.Name() != "apt" {
                t.Errorf("Name() = %q, want %q", pm.Name(), "apt")
        }
}

func TestNewInstaller_ArchFamily(t *testing.T) {
        t.Parallel()

        pm, err := NewInstaller("arch", simpleMockExec())
        if err != nil {
                t.Fatalf("NewInstaller(arch) failed: %v", err)
        }
        if pm.Name() != "pacman" {
                t.Errorf("Name() = %q, want %q", pm.Name(), "pacman")
        }
}

func TestNewInstaller_FedoraFamily(t *testing.T) {
        t.Parallel()

        pm, err := NewInstaller("fedora", simpleMockExec())
        if err != nil {
                t.Fatalf("NewInstaller(fedora) failed: %v", err)
        }
        if pm.Name() != "dnf" {
                t.Errorf("Name() = %q, want %q", pm.Name(), "dnf")
        }
}

func TestNewInstaller_AlpineFamily(t *testing.T) {
        t.Parallel()

        pm, err := NewInstaller("alpine", simpleMockExec())
        if err != nil {
                t.Fatalf("NewInstaller(alpine) failed: %v", err)
        }
        if pm.Name() != "apk" {
                t.Errorf("Name() = %q, want %q", pm.Name(), "apk")
        }
}

func TestNewInstaller_UnknownFamily(t *testing.T) {
        t.Parallel()

        _, err := NewInstaller("unknown", simpleMockExec())
        if err == nil {
                t.Fatal("NewInstaller with unknown family should return an error")
        }
        if !contains(err.Error(), "unsupported") {
                t.Errorf("error should mention 'unsupported', got: %v", err)
        }
}

func TestNewInstaller_NilExecFunc(t *testing.T) {
        t.Parallel()

        _, err := NewInstaller("debian", nil)
        if err == nil {
                t.Fatal("NewInstaller with nil ExecFunc should return an error")
        }
        if !contains(err.Error(), "SECURITY") {
                t.Errorf("error should mention SECURITY, got: %v", err)
        }
}

// ---------------------------------------------------------------------------
// ClassifyPriority — priority classification tests
// ---------------------------------------------------------------------------

func TestClassifyPriority(t *testing.T) {
        t.Parallel()

        tests := []struct {
                name     string
                pkg      string
                expected int
        }{
                // Foundation packages
                {"ca_certificates", "ca-certificates", PriorityFoundation},
                {"gnupg", "gnupg", PriorityFoundation},
                {"gnupg2", "gnupg2", PriorityFoundation},
                {"build_essential", "build-essential", PriorityFoundation},
                {"base_devel", "base-devel", PriorityFoundation},
                {"build_base", "build-base", PriorityFoundation},
                {"gcc", "gcc", PriorityFoundation},

                // Language packages
                {"python3", "python3", PriorityLanguage},
                {"python", "python", PriorityLanguage},
                {"python3_pip", "python3-pip", PriorityLanguage},
                {"py3_pip", "py3-pip", PriorityLanguage},
                {"python_pip", "python-pip", PriorityLanguage},
                {"pip", "pip", PriorityLanguage},
                {"openjdk21_jdk", "openjdk-21-jdk", PriorityLanguage},
                {"jdk_openjdk", "jdk-openjdk", PriorityLanguage},
                {"java21_openjdk", "java-21-openjdk-devel", PriorityLanguage},
                {"openjdk21", "openjdk21", PriorityLanguage},
                {"nodejs", "nodejs", PriorityLanguage},
                {"npm", "npm", PriorityLanguage},

                // Tool packages (everything else)
                {"git", "git", PriorityTool},
                {"curl", "curl", PriorityTool},
                {"vim", "vim", PriorityTool},
                {"htop", "htop", PriorityTool},
                {"tmux", "tmux", PriorityTool},
                {"zsh", "zsh", PriorityTool},
                {"wget", "wget", PriorityTool},
                {"docker", "docker", PriorityTool},
                {"unknown_package", "some-random-package", PriorityTool},
                {"empty_string", "", PriorityTool},
        }

        for _, tt := range tests {
                tt := tt
                t.Run(tt.name, func(t *testing.T) {
                        t.Parallel()
                        got := ClassifyPriority(tt.pkg)
                        if got != tt.expected {
                                t.Errorf("ClassifyPriority(%q) = %d, want %d", tt.pkg, got, tt.expected)
                        }
                })
        }
}

func TestPriorityConstants(t *testing.T) {
        t.Parallel()

        if PriorityFoundation != 1 {
                t.Errorf("PriorityFoundation = %d, want 1", PriorityFoundation)
        }
        if PriorityLanguage != 2 {
                t.Errorf("PriorityLanguage = %d, want 2", PriorityLanguage)
        }
        if PriorityTool != 3 {
                t.Errorf("PriorityTool = %d, want 3", PriorityTool)
        }

        // Foundation must have the highest priority (lowest number)
        if PriorityFoundation >= PriorityLanguage {
                t.Error("PriorityFoundation should be < PriorityLanguage")
        }
        if PriorityLanguage >= PriorityTool {
                t.Error("PriorityLanguage should be < PriorityTool")
        }
}

// ---------------------------------------------------------------------------
// ExecFunc type — contract verification
// ---------------------------------------------------------------------------

func TestExecFunc_Contract(t *testing.T) {
        t.Parallel()

        ctx := context.Background()

        // Verify the ExecFunc type signature works as expected
        fn := func(ctx context.Context, command string, args ...string) (string, error) {
                return "output", nil
        }

        // Should be assignable to ExecFunc
        var execFn ExecFunc = fn
        output, err := execFn(ctx, "test", "arg1", "arg2")
        if err != nil {
                t.Fatalf("ExecFunc call failed: %v", err)
        }
        if output != "output" {
                t.Errorf("output = %q, want %q", output, "output")
        }
}

// ---------------------------------------------------------------------------
// AptInstaller — mock-based tests
// ---------------------------------------------------------------------------

func TestAptInstaller_Install(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo apt-get install -y git":  {"Setting up git...", nil},
                "sudo apt-get install -y curl": {"Setting up curl...", nil},
        })

        pm, _ := NewInstaller("debian", mock)
        results, err := pm.Install(ctx, []string{"git", "curl"})

        if err != nil {
                t.Fatalf("Install failed: %v", err)
        }
        if len(results) != 2 {
                t.Fatalf("expected 2 results, got %d", len(results))
        }
        for _, r := range results {
                if !r.Success {
                        t.Errorf("package %q should have succeeded", r.Package)
                }
        }
}

func TestAptInstaller_InstallFailure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo apt-get install -y nonexistent": {
                        "", fmt.Errorf("EXEC: command 'sudo' failed: exit status 100 (stderr: Unable to locate package nonexistent)"),
                },
        })

        pm, _ := NewInstaller("debian", mock)
        results, _ := pm.Install(ctx, []string{"nonexistent"})

        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if results[0].Success {
                t.Error("nonexistent package should fail")
        }
        if results[0].Error != "PACKAGE_NOT_FOUND" {
                t.Errorf("Error = %q, want %q", results[0].Error, "PACKAGE_NOT_FOUND")
        }
}

func TestAptInstaller_IsInstalled_True(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "dpkg -s git": {"Status: install ok installed\nPackage: git\n", nil},
        })

        pm, _ := NewInstaller("debian", mock)
        if !pm.IsInstalled(ctx, "git") {
                t.Error("git should be reported as installed")
        }
}

func TestAptInstaller_IsInstalled_False(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "dpkg -s nonexistent": {"", fmt.Errorf("not installed")},
        })

        pm, _ := NewInstaller("debian", mock)
        if pm.IsInstalled(ctx, "nonexistent") {
                t.Error("nonexistent should not be reported as installed")
        }
}

func TestAptInstaller_RefreshIndex(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo apt-get update": {"Reading package lists... Done", nil},
        })

        pm, _ := NewInstaller("debian", mock)
        err := pm.RefreshIndex(ctx)
        if err != nil {
                t.Fatalf("RefreshIndex failed: %v", err)
        }
}

func TestAptInstaller_Remove(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo apt-get remove -y git": {"Removing git...", nil},
        })

        pm, _ := NewInstaller("debian", mock)
        results, err := pm.Remove(ctx, []string{"git"})
        if err != nil {
                t.Fatalf("Remove failed: %v", err)
        }
        if len(results) != 1 || !results[0].Success {
                t.Error("remove should succeed")
        }
}

func TestAptInstaller_Search(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apt-cache search git": {"git - fast, scalable, distributed revision control system\ngitk - graphical repository viewer", nil},
        })

        pm, _ := NewInstaller("debian", mock)
        results, err := pm.Search(ctx, "git")
        if err != nil {
                t.Fatalf("Search failed: %v", err)
        }
        if len(results) != 2 {
                t.Errorf("expected 2 results, got %d", len(results))
        }
        if results[0] != "git" {
                t.Errorf("first result = %q, want %q", results[0], "git")
        }
}

// ---------------------------------------------------------------------------
// PacmanInstaller — mock-based tests
// ---------------------------------------------------------------------------

func TestPacmanInstaller_Name(t *testing.T) {
        t.Parallel()

        pm, _ := NewInstaller("arch", simpleMockExec())
        if pm.Name() != "pacman" {
                t.Errorf("Name() = %q, want %q", pm.Name(), "pacman")
        }
}

func TestPacmanInstaller_Install(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo pacman -S --noconfirm vim": {"installing vim...", nil},
        })

        pm, _ := NewInstaller("arch", mock)
        results, _ := pm.Install(ctx, []string{"vim"})
        if len(results) != 1 || !results[0].Success {
                t.Error("pacman install should succeed")
        }
}

// ---------------------------------------------------------------------------
// DnfInstaller — mock-based tests
// ---------------------------------------------------------------------------

func TestDnfInstaller_Name(t *testing.T) {
        t.Parallel()

        pm, _ := NewInstaller("fedora", simpleMockExec())
        if pm.Name() != "dnf" {
                t.Errorf("Name() = %q, want %q", pm.Name(), "dnf")
        }
}

func TestDnfInstaller_Install(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo dnf install -y htop": {"Installing htop...", nil},
        })

        pm, _ := NewInstaller("fedora", mock)
        results, _ := pm.Install(ctx, []string{"htop"})
        if len(results) != 1 || !results[0].Success {
                t.Error("dnf install should succeed")
        }
}

// ---------------------------------------------------------------------------
// ApkInstaller — mock-based tests
// ---------------------------------------------------------------------------

func TestApkInstaller_Name(t *testing.T) {
        t.Parallel()

        pm, _ := NewInstaller("alpine", simpleMockExec())
        if pm.Name() != "apk" {
                t.Errorf("Name() = %q, want %q", pm.Name(), "apk")
        }
}

func TestApkInstaller_Install(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk add tmux": {"Installing tmux...", nil},
        })

        pm, _ := NewInstaller("alpine", mock)
        results, _ := pm.Install(ctx, []string{"tmux"})
        if len(results) != 1 || !results[0].Success {
                t.Error("apk install should succeed")
        }
}

func TestApkInstaller_IsInstalled(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk info -e tmux": {"tmux-3.4", nil},
                "apk info -e nonexistent": {"", fmt.Errorf("not found")},
        })

        pm, _ := NewInstaller("alpine", mock)

        if !pm.IsInstalled(ctx, "tmux") {
                t.Error("tmux should be installed")
        }
        if pm.IsInstalled(ctx, "nonexistent") {
                t.Error("nonexistent should not be installed")
        }
}

// ---------------------------------------------------------------------------
// classifyAptError — error classification tests
// ---------------------------------------------------------------------------

func TestClassifyAptError(t *testing.T) {
        t.Parallel()

        tests := []struct {
                name     string
                errMsg   string
                expected string
        }{
                {"unable_to_locate", "Unable to locate package foo", "PACKAGE_NOT_FOUND"},
                {"dpkg_interrupted", "dpkg was interrupted", "SYSTEM_BROKEN"},
                {"permission_denied", "Permission denied", "NO_SUDO"},
                {"lock_held", "Could not get lock", "LOCK_HELD"},
                {"unmet_deps", "Unmet dependencies", "DEPENDENCY_ERROR"},
                {"disk_full", "No space left on device", "DISK_FULL"},
                {"failed_to_fetch", "Failed to fetch http://repo", "NETWORK_ERROR"},
                {"unknown_error", "Something went wrong", "UNKNOWN"},
        }

        for _, tt := range tests {
                tt := tt
                t.Run(tt.name, func(t *testing.T) {
                        t.Parallel()
                        got := classifyAptError(tt.errMsg)
                        if got != tt.expected {
                                t.Errorf("classifyAptError(%q) = %q, want %q", tt.errMsg, got, tt.expected)
                        }
                })
        }
}

// ---------------------------------------------------------------------------
// classifyPacmanError — error classification tests
// ---------------------------------------------------------------------------

func TestClassifyPacmanError(t *testing.T) {
        t.Parallel()

        tests := []struct {
                name     string
                errMsg   string
                expected string
        }{
                {"target_not_found", "target not found: foo", "PACKAGE_NOT_FOUND"},
                {"permission_denied", "permission denied", "NO_SUDO"},
                {"transaction_error", "failed to commit transaction", "TRANSACTION_ERROR"},
                {"file_conflict", "exists in filesystem", "FILE_CONFLICT"},
                {"unknown_error", "something else", "UNKNOWN"},
        }

        for _, tt := range tests {
                tt := tt
                t.Run(tt.name, func(t *testing.T) {
                        t.Parallel()
                        got := classifyPacmanError(tt.errMsg)
                        if got != tt.expected {
                                t.Errorf("classifyPacmanError(%q) = %q, want %q", tt.errMsg, got, tt.expected)
                        }
                })
        }
}

// ---------------------------------------------------------------------------
// classifyDnfError — error classification tests
// ---------------------------------------------------------------------------

func TestClassifyDnfError(t *testing.T) {
        t.Parallel()

        tests := []struct {
                name     string
                errMsg   string
                expected string
        }{
                {"no_match", "No match for argument: foo", "PACKAGE_NOT_FOUND"},
                {"permission_denied", "Permission denied", "NO_SUDO"},
                {"already_installed", "already installed", "ALREADY_INSTALLED"},
                {"unknown_error", "something else", "UNKNOWN"},
        }

        for _, tt := range tests {
                tt := tt
                t.Run(tt.name, func(t *testing.T) {
                        t.Parallel()
                        got := classifyDnfError(tt.errMsg)
                        if got != tt.expected {
                                t.Errorf("classifyDnfError(%q) = %q, want %q", tt.errMsg, got, tt.expected)
                        }
                })
        }
}

// ---------------------------------------------------------------------------
// classifyApkError — error classification tests
// ---------------------------------------------------------------------------

func TestClassifyApkError(t *testing.T) {
        t.Parallel()

        tests := []struct {
                name     string
                errMsg   string
                expected string
        }{
                {"could_not_find", "could not find package", "PACKAGE_NOT_FOUND"},
                {"permission_denied", "permission denied", "NO_SUDO"},
                {"unknown_error", "something else", "UNKNOWN"},
        }

        for _, tt := range tests {
                tt := tt
                t.Run(tt.name, func(t *testing.T) {
                        t.Parallel()
                        got := classifyApkError(tt.errMsg)
                        if got != tt.expected {
                                t.Errorf("classifyApkError(%q) = %q, want %q", tt.errMsg, got, tt.expected)
                        }
                })
        }
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func contains(s, substr string) bool {
        return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
        for i := 0; i <= len(s)-len(substr); i++ {
                if s[i:i+len(substr)] == substr {
                        return true
                }
        }
        return false
}

// ---------------------------------------------------------------------------
// BuildDependencyMap — dependency map construction tests
// ---------------------------------------------------------------------------

func TestBuildDependencyMap_Empty(t *testing.T) {
        t.Parallel()

        result := BuildDependencyMap(map[string]engine.PackageState{})
        if len(result) != 0 {
                t.Errorf("expected empty map, got %d entries", len(result))
        }
}

func TestBuildDependencyMap_Nil(t *testing.T) {
        t.Parallel()

        result := BuildDependencyMap(nil)
        if len(result) != 0 {
                t.Errorf("expected empty map for nil input, got %d entries", len(result))
        }
}

func TestBuildDependencyMap_ToolsOnly(t *testing.T) {
        t.Parallel()

        managed := map[string]engine.PackageState{
                "git":  {PackageManager: "apt", Verified: true},
                "curl": {PackageManager: "apt", Verified: true},
                "vim":  {PackageManager: "apt", Verified: true},
        }

        result := BuildDependencyMap(managed)

        // Tools don't depend on other tools, so the map should be empty
        if len(result) != 0 {
                t.Errorf("expected empty dependency map for tools-only, got %d entries", len(result))
        }
}

func TestBuildDependencyMap_FoundationAndLanguage(t *testing.T) {
        t.Parallel()

        managed := map[string]engine.PackageState{
                "ca-certificates": {PackageManager: "apt", Verified: true},
                "gnupg":           {PackageManager: "apt", Verified: true},
                "python3":         {PackageManager: "apt", Verified: true},
                "nodejs":          {PackageManager: "apt", Verified: true},
        }

        result := BuildDependencyMap(managed)

        // Both python3 and nodejs should depend on both ca-certificates and gnupg
        if len(result) != 2 {
                t.Fatalf("expected 2 foundation packages in dependency map, got %d", len(result))
        }

        // Check ca-certificates dependents
        caDeps := result["ca-certificates"]
        if len(caDeps) != 2 {
                t.Errorf("ca-certificates should have 2 dependents, got %d: %v", len(caDeps), caDeps)
        }

        // Check gnupg dependents
        gnupgDeps := result["gnupg"]
        if len(gnupgDeps) != 2 {
                t.Errorf("gnupg should have 2 dependents, got %d: %v", len(gnupgDeps), gnupgDeps)
        }

        // Tools should not be in the dependency map as keys
        if _, exists := result["git"]; exists {
                t.Error("tool packages should not be keys in dependency map")
        }
}

func TestBuildDependencyMap_MixedPriorities(t *testing.T) {
        t.Parallel()

        managed := map[string]engine.PackageState{
                "ca-certificates": {PackageManager: "apt", Verified: true},
                "python3":         {PackageManager: "apt", Verified: true},
                "git":             {PackageManager: "apt", Verified: true},
                "vim":             {PackageManager: "apt", Verified: true},
        }

        result := BuildDependencyMap(managed)

        // Only ca-certificates should be a key (foundation)
        if len(result) != 1 {
                t.Fatalf("expected 1 key in dependency map, got %d", len(result))
        }

        caDeps := result["ca-certificates"]
        if len(caDeps) != 1 {
                t.Errorf("ca-certificates should have 1 dependent (python3), got %d: %v", len(caDeps), caDeps)
        }

        // git and vim should not appear as dependents (they are tools)
        foundGit := false
        for _, dep := range caDeps {
                if dep == "git" {
                        foundGit = true
                }
        }
        if foundGit {
                t.Error("git (a tool) should not be listed as a dependent of a foundation package")
        }
}

func TestBuildDependencyMap_FoundationOnly(t *testing.T) {
        t.Parallel()

        managed := map[string]engine.PackageState{
                "ca-certificates": {PackageManager: "apt", Verified: true},
                "gnupg":           {PackageManager: "apt", Verified: true},
        }

        result := BuildDependencyMap(managed)

        // No language packages to depend on foundations
        if len(result) != 0 {
                t.Errorf("expected empty dependency map for foundation-only, got %d entries", len(result))
        }
}

func TestBuildDependencyMap_LanguageOnly(t *testing.T) {
        t.Parallel()

        managed := map[string]engine.PackageState{
                "python3": {PackageManager: "apt", Verified: true},
                "nodejs":  {PackageManager: "apt", Verified: true},
        }

        result := BuildDependencyMap(managed)

        // No foundations for languages to depend on
        if len(result) != 0 {
                t.Errorf("expected empty dependency map for language-only, got %d entries", len(result))
        }
}

// ---------------------------------------------------------------------------
// AptInstaller — additional CRUD method tests
// ---------------------------------------------------------------------------

func TestAptInstaller_Update(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo apt-get install --only-upgrade -y git": {"Setting up git (1:2.40.0-1)...", nil},
        })

        pm, _ := NewInstaller("debian", mock)
        results, err := pm.Update(ctx, []string{"git"})

        if err != nil {
                t.Fatalf("Update failed: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if !results[0].Success {
                t.Errorf("update should succeed, got error: %s", results[0].Error)
        }
        if results[0].Package != "git" {
                t.Errorf("Package = %q, want %q", results[0].Package, "git")
        }
        if results[0].Action != "update" {
                t.Errorf("Action = %q, want %q", results[0].Action, "update")
        }
}

func TestAptInstaller_Update_Failure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo apt-get install --only-upgrade -y nonexistent": {
                        "", fmt.Errorf("Unable to locate package nonexistent"),
                },
        })

        pm, _ := NewInstaller("debian", mock)
        results, err := pm.Update(ctx, []string{"nonexistent"})

        if err != nil {
                t.Fatalf("Update should not return error at function level: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if results[0].Success {
                t.Error("update of nonexistent package should fail")
        }
        if results[0].Error == "" {
                t.Error("Error should not be empty on failure")
        }
}

func TestAptInstaller_ListInstalled(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "dpkg --get-selections": {
                        "git\tinstall\ncurl\tinstall\nvim\tinstall\nlinux-headers\tdeinstall\n",
                        nil,
                },
        })

        pm, _ := NewInstaller("debian", mock)
        packages, err := pm.ListInstalled(ctx)

        if err != nil {
                t.Fatalf("ListInstalled failed: %v", err)
        }
        // Should only include packages with "install" status, not "deinstall"
        expected := []string{"git", "curl", "vim"}
        if len(packages) != len(expected) {
                t.Fatalf("expected %d packages, got %d", len(expected), len(packages))
        }
        for i, pkg := range expected {
                if packages[i] != pkg {
                        t.Errorf("packages[%d] = %q, want %q", i, packages[i], pkg)
                }
        }
}

func TestAptInstaller_ListInstalled_Error(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "dpkg --get-selections": {"", fmt.Errorf("dpkg: error: unable to access")},
        })

        pm, _ := NewInstaller("debian", mock)
        packages, err := pm.ListInstalled(ctx)

        if err == nil {
                t.Fatal("ListInstalled should return an error when dpkg fails")
        }
        if packages != nil {
                t.Errorf("expected nil packages on error, got %v", packages)
        }
}

func TestAptInstaller_Remove_Failure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo apt-get remove -y nonexistent": {
                        "", fmt.Errorf("E: Unable to locate package nonexistent"),
                },
        })

        pm, _ := NewInstaller("debian", mock)
        results, err := pm.Remove(ctx, []string{"nonexistent"})

        if err != nil {
                t.Fatalf("Remove should not return error at function level: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if results[0].Success {
                t.Error("remove of nonexistent package should fail")
        }
        if results[0].Error == "" {
                t.Error("Error should not be empty on failure")
        }
}

func TestAptInstaller_Search_EmptyResults(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apt-cache search zzznotfound": {"", nil},
        })

        pm, _ := NewInstaller("debian", mock)
        results, err := pm.Search(ctx, "zzznotfound")

        if err != nil {
                t.Fatalf("Search failed: %v", err)
        }
        if len(results) != 0 {
                t.Errorf("expected 0 results for empty output, got %d", len(results))
        }
}

func TestAptInstaller_Search_Error(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apt-cache search git": {"", fmt.Errorf("network unreachable")},
        })

        pm, _ := NewInstaller("debian", mock)
        results, err := pm.Search(ctx, "git")

        if err == nil {
                t.Fatal("Search should return an error when apt-cache fails")
        }
        if results != nil {
                t.Errorf("expected nil results on error, got %v", results)
        }
}

// ---------------------------------------------------------------------------
// PacmanInstaller — comprehensive CRUD method tests
// ---------------------------------------------------------------------------

func TestPacmanInstaller_RefreshIndex(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo pacman -Sy": {":: Synchronizing package databases...", nil},
        })

        pm, _ := NewInstaller("arch", mock)
        err := pm.RefreshIndex(ctx)

        if err != nil {
                t.Fatalf("RefreshIndex failed: %v", err)
        }
}

func TestPacmanInstaller_RefreshIndex_Error(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo pacman -Sy": {"", fmt.Errorf("failed to synchronize databases")},
        })

        pm, _ := NewInstaller("arch", mock)
        err := pm.RefreshIndex(ctx)

        if err == nil {
                t.Fatal("RefreshIndex should return an error when pacman -Sy fails")
        }
}

func TestPacmanInstaller_Remove(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo pacman -R --noconfirm vim": {"removing vim...", nil},
        })

        pm, _ := NewInstaller("arch", mock)
        results, err := pm.Remove(ctx, []string{"vim"})

        if err != nil {
                t.Fatalf("Remove failed: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if !results[0].Success {
                t.Errorf("remove should succeed, got error: %s", results[0].Error)
        }
        if results[0].Package != "vim" {
                t.Errorf("Package = %q, want %q", results[0].Package, "vim")
        }
        if results[0].Action != "remove" {
                t.Errorf("Action = %q, want %q", results[0].Action, "remove")
        }
}

func TestPacmanInstaller_Remove_Failure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo pacman -R --noconfirm nonexistent": {
                        "", fmt.Errorf("target not found: nonexistent"),
                },
        })

        pm, _ := NewInstaller("arch", mock)
        results, err := pm.Remove(ctx, []string{"nonexistent"})

        if err != nil {
                t.Fatalf("Remove should not return error at function level: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if results[0].Success {
                t.Error("remove of nonexistent package should fail")
        }
}

func TestPacmanInstaller_Update(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo pacman -S --noconfirm vim": {"upgrading vim...", nil},
        })

        pm, _ := NewInstaller("arch", mock)
        results, err := pm.Update(ctx, []string{"vim"})

        if err != nil {
                t.Fatalf("Update failed: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if !results[0].Success {
                t.Errorf("update should succeed, got error: %s", results[0].Error)
        }
        if results[0].Package != "vim" {
                t.Errorf("Package = %q, want %q", results[0].Package, "vim")
        }
        if results[0].Action != "update" {
                t.Errorf("Action = %q, want %q", results[0].Action, "update")
        }
}

func TestPacmanInstaller_Update_Failure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo pacman -S --noconfirm nonexistent": {
                        "", fmt.Errorf("target not found: nonexistent"),
                },
        })

        pm, _ := NewInstaller("arch", mock)
        results, err := pm.Update(ctx, []string{"nonexistent"})

        if err != nil {
                t.Fatalf("Update should not return error at function level: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if results[0].Success {
                t.Error("update of nonexistent package should fail")
        }
}

func TestPacmanInstaller_IsInstalled_True(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "pacman -Q vim": {"vim 9.0.2050-1", nil},
        })

        pm, _ := NewInstaller("arch", mock)
        if !pm.IsInstalled(ctx, "vim") {
                t.Error("vim should be reported as installed")
        }
}

func TestPacmanInstaller_IsInstalled_False(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "pacman -Q nonexistent": {"", fmt.Errorf("package not found")},
        })

        pm, _ := NewInstaller("arch", mock)
        if pm.IsInstalled(ctx, "nonexistent") {
                t.Error("nonexistent should not be reported as installed")
        }
}

func TestPacmanInstaller_ListInstalled(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "pacman -Qe": {"vim 9.0.2050-1\ngit 2.42.0-1\ncurl 8.4.0-2\n", nil},
        })

        pm, _ := NewInstaller("arch", mock)
        packages, err := pm.ListInstalled(ctx)

        if err != nil {
                t.Fatalf("ListInstalled failed: %v", err)
        }
        expected := []string{"vim", "git", "curl"}
        if len(packages) != len(expected) {
                t.Fatalf("expected %d packages, got %d", len(expected), len(packages))
        }
        for i, pkg := range expected {
                if packages[i] != pkg {
                        t.Errorf("packages[%d] = %q, want %q", i, packages[i], pkg)
                }
        }
}

func TestPacmanInstaller_ListInstalled_Error(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "pacman -Qe": {"", fmt.Errorf("pacman: error reading database")},
        })

        pm, _ := NewInstaller("arch", mock)
        packages, err := pm.ListInstalled(ctx)

        if err == nil {
                t.Fatal("ListInstalled should return an error when pacman -Qe fails")
        }
        if packages != nil {
                t.Errorf("expected nil packages on error, got %v", packages)
        }
}

func TestPacmanInstaller_Search(t *testing.T) {
        ctx := context.Background()

        // pacman -Ss output format: repo/package version on first line,
        // indented description on second line
        mock := mockExecFunc(map[string]mockResponse{
                "pacman -Ss vim": {
                        "extra/vim 9.0.2050-1\n  Vi Improved, a highly configurable text editor\nextra/vim-runtime 9.0.2050-1\n  Runtime files for vim\n",
                        nil,
                },
        })

        pm, _ := NewInstaller("arch", mock)
        results, err := pm.Search(ctx, "vim")

        if err != nil {
                t.Fatalf("Search failed: %v", err)
        }
        expected := []string{"vim", "vim-runtime"}
        if len(results) != len(expected) {
                t.Fatalf("expected %d results, got %d", len(expected), len(results))
        }
        for i, pkg := range expected {
                if results[i] != pkg {
                        t.Errorf("results[%d] = %q, want %q", i, results[i], pkg)
                }
        }
}

func TestPacmanInstaller_Search_Error(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "pacman -Ss vim": {"", fmt.Errorf("failed to synchronize databases")},
        })

        pm, _ := NewInstaller("arch", mock)
        results, err := pm.Search(ctx, "vim")

        if err == nil {
                t.Fatal("Search should return an error when pacman -Ss fails")
        }
        if results != nil {
                t.Errorf("expected nil results on error, got %v", results)
        }
}

// ---------------------------------------------------------------------------
// DnfInstaller — comprehensive CRUD method tests
// ---------------------------------------------------------------------------

func TestDnfInstaller_RefreshIndex(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo dnf makecache": {"Metadata cache created.", nil},
        })

        pm, _ := NewInstaller("fedora", mock)
        err := pm.RefreshIndex(ctx)

        if err != nil {
                t.Fatalf("RefreshIndex failed: %v", err)
        }
}

func TestDnfInstaller_RefreshIndex_Error(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo dnf makecache": {"", fmt.Errorf("Failed to download metadata")},
        })

        pm, _ := NewInstaller("fedora", mock)
        err := pm.RefreshIndex(ctx)

        if err == nil {
                t.Fatal("RefreshIndex should return an error when dnf makecache fails")
        }
}

func TestDnfInstaller_Remove(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo dnf remove -y htop": {"Removed htop.", nil},
        })

        pm, _ := NewInstaller("fedora", mock)
        results, err := pm.Remove(ctx, []string{"htop"})

        if err != nil {
                t.Fatalf("Remove failed: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if !results[0].Success {
                t.Errorf("remove should succeed, got error: %s", results[0].Error)
        }
        if results[0].Package != "htop" {
                t.Errorf("Package = %q, want %q", results[0].Package, "htop")
        }
        if results[0].Action != "remove" {
                t.Errorf("Action = %q, want %q", results[0].Action, "remove")
        }
}

func TestDnfInstaller_Remove_Failure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo dnf remove -y nonexistent": {
                        "", fmt.Errorf("No match for argument: nonexistent"),
                },
        })

        pm, _ := NewInstaller("fedora", mock)
        results, err := pm.Remove(ctx, []string{"nonexistent"})

        if err != nil {
                t.Fatalf("Remove should not return error at function level: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if results[0].Success {
                t.Error("remove of nonexistent package should fail")
        }
}

func TestDnfInstaller_Update(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo dnf upgrade -y htop": {"Upgrading htop...", nil},
        })

        pm, _ := NewInstaller("fedora", mock)
        results, err := pm.Update(ctx, []string{"htop"})

        if err != nil {
                t.Fatalf("Update failed: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if !results[0].Success {
                t.Errorf("update should succeed, got error: %s", results[0].Error)
        }
        if results[0].Package != "htop" {
                t.Errorf("Package = %q, want %q", results[0].Package, "htop")
        }
        if results[0].Action != "update" {
                t.Errorf("Action = %q, want %q", results[0].Action, "update")
        }
}

func TestDnfInstaller_Update_Failure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "sudo dnf upgrade -y nonexistent": {
                        "", fmt.Errorf("No match for argument: nonexistent"),
                },
        })

        pm, _ := NewInstaller("fedora", mock)
        results, err := pm.Update(ctx, []string{"nonexistent"})

        if err != nil {
                t.Fatalf("Update should not return error at function level: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if results[0].Success {
                t.Error("update of nonexistent package should fail")
        }
}

func TestDnfInstaller_IsInstalled_True(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "rpm -q htop": {"htop-3.2.2-4.fc38.x86_64", nil},
        })

        pm, _ := NewInstaller("fedora", mock)
        if !pm.IsInstalled(ctx, "htop") {
                t.Error("htop should be reported as installed")
        }
}

func TestDnfInstaller_IsInstalled_False(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "rpm -q nonexistent": {"", fmt.Errorf("package nonexistent is not installed")},
        })

        pm, _ := NewInstaller("fedora", mock)
        if pm.IsInstalled(ctx, "nonexistent") {
                t.Error("nonexistent should not be reported as installed")
        }
}

func TestDnfInstaller_ListInstalled(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "rpm -qa": {"htop-3.2.2-4.fc38.x86_64\ngit-2.42.0-1.fc38.x86_64\ncurl-8.4.0-1.fc38.x86_64\n", nil},
        })

        pm, _ := NewInstaller("fedora", mock)
        packages, err := pm.ListInstalled(ctx)

        if err != nil {
                t.Fatalf("ListInstalled failed: %v", err)
        }
        expected := []string{"htop-3.2.2-4.fc38.x86_64", "git-2.42.0-1.fc38.x86_64", "curl-8.4.0-1.fc38.x86_64"}
        if len(packages) != len(expected) {
                t.Fatalf("expected %d packages, got %d", len(expected), len(packages))
        }
        for i, pkg := range expected {
                if packages[i] != pkg {
                        t.Errorf("packages[%d] = %q, want %q", i, packages[i], pkg)
                }
        }
}

func TestDnfInstaller_ListInstalled_Error(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "rpm -qa": {"", fmt.Errorf("rpm: error reading database")},
        })

        pm, _ := NewInstaller("fedora", mock)
        packages, err := pm.ListInstalled(ctx)

        if err == nil {
                t.Fatal("ListInstalled should return an error when rpm -qa fails")
        }
        if packages != nil {
                t.Errorf("expected nil packages on error, got %v", packages)
        }
}

func TestDnfInstaller_Search(t *testing.T) {
        ctx := context.Background()

        // dnf search output with metadata lines that should be filtered
        mock := mockExecFunc(map[string]mockResponse{
                "dnf search vim": {
                        "Last metadata expiration check: 0:01:00 ago.\nvim-enhanced.x86_64 : A version of the VIM editor\nvim-minimal.x86_64 : A minimal version of the VIM editor\n======================== Name Exactly Matched: vim ========================\n",
                        nil,
                },
        })

        pm, _ := NewInstaller("fedora", mock)
        results, err := pm.Search(ctx, "vim")

        if err != nil {
                t.Fatalf("Search failed: %v", err)
        }
        // Should filter out "Last metadata" and "=" lines, then extract package name before "."
        if len(results) < 2 {
                t.Fatalf("expected at least 2 results, got %d", len(results))
        }
        // First result should be "vim-enhanced" (before .arch)
        if results[0] != "vim-enhanced" {
                t.Errorf("results[0] = %q, want %q", results[0], "vim-enhanced")
        }
        if results[1] != "vim-minimal" {
                t.Errorf("results[1] = %q, want %q", results[1], "vim-minimal")
        }
        // Metadata lines should be filtered out
        for _, r := range results {
                if r == "" {
                        t.Error("result should not be empty")
                }
        }
}

func TestDnfInstaller_Search_Error(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "dnf search vim": {"", fmt.Errorf("failed to synchronize repos")},
        })

        pm, _ := NewInstaller("fedora", mock)
        results, err := pm.Search(ctx, "vim")

        if err == nil {
                t.Fatal("Search should return an error when dnf search fails")
        }
        if results != nil {
                t.Errorf("expected nil results on error, got %v", results)
        }
}

// ---------------------------------------------------------------------------
// ApkInstaller — comprehensive CRUD method tests
// ---------------------------------------------------------------------------

func TestApkInstaller_RefreshIndex(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk update": {"fetch https://dl-cdn.alpinelinux.org/alpine/v3.19/main\nOK: 1", nil},
        })

        pm, _ := NewInstaller("alpine", mock)
        err := pm.RefreshIndex(ctx)

        if err != nil {
                t.Fatalf("RefreshIndex failed: %v", err)
        }
}

func TestApkInstaller_RefreshIndex_Error(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk update": {"", fmt.Errorf("network unreachable")},
        })

        pm, _ := NewInstaller("alpine", mock)
        err := pm.RefreshIndex(ctx)

        if err == nil {
                t.Fatal("RefreshIndex should return an error when apk update fails")
        }
}

func TestApkInstaller_Remove(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk del tmux": {"OK: 1", nil},
        })

        pm, _ := NewInstaller("alpine", mock)
        results, err := pm.Remove(ctx, []string{"tmux"})

        if err != nil {
                t.Fatalf("Remove failed: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if !results[0].Success {
                t.Errorf("remove should succeed, got error: %s", results[0].Error)
        }
        if results[0].Package != "tmux" {
                t.Errorf("Package = %q, want %q", results[0].Package, "tmux")
        }
        if results[0].Action != "remove" {
                t.Errorf("Action = %q, want %q", results[0].Action, "remove")
        }
}

func TestApkInstaller_Remove_Failure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk del nonexistent": {
                        "", fmt.Errorf("could not find package nonexistent"),
                },
        })

        pm, _ := NewInstaller("alpine", mock)
        results, err := pm.Remove(ctx, []string{"nonexistent"})

        if err != nil {
                t.Fatalf("Remove should not return error at function level: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if results[0].Success {
                t.Error("remove of nonexistent package should fail")
        }
}

func TestApkInstaller_Update(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk upgrade tmux": {"OK: Upgrading tmux...", nil},
        })

        pm, _ := NewInstaller("alpine", mock)
        results, err := pm.Update(ctx, []string{"tmux"})

        if err != nil {
                t.Fatalf("Update failed: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if !results[0].Success {
                t.Errorf("update should succeed, got error: %s", results[0].Error)
        }
        if results[0].Package != "tmux" {
                t.Errorf("Package = %q, want %q", results[0].Package, "tmux")
        }
        if results[0].Action != "update" {
                t.Errorf("Action = %q, want %q", results[0].Action, "update")
        }
}

func TestApkInstaller_Update_Failure(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk upgrade nonexistent": {
                        "", fmt.Errorf("could not find package nonexistent"),
                },
        })

        pm, _ := NewInstaller("alpine", mock)
        results, err := pm.Update(ctx, []string{"nonexistent"})

        if err != nil {
                t.Fatalf("Update should not return error at function level: %v", err)
        }
        if len(results) != 1 {
                t.Fatalf("expected 1 result, got %d", len(results))
        }
        if results[0].Success {
                t.Error("update of nonexistent package should fail")
        }
}

func TestApkInstaller_ListInstalled(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk info": {"tmux\ncurl\nvim\ngit\n", nil},
        })

        pm, _ := NewInstaller("alpine", mock)
        packages, err := pm.ListInstalled(ctx)

        if err != nil {
                t.Fatalf("ListInstalled failed: %v", err)
        }
        expected := []string{"tmux", "curl", "vim", "git"}
        if len(packages) != len(expected) {
                t.Fatalf("expected %d packages, got %d", len(expected), len(packages))
        }
        for i, pkg := range expected {
                if packages[i] != pkg {
                        t.Errorf("packages[%d] = %q, want %q", i, packages[i], pkg)
                }
        }
}

func TestApkInstaller_ListInstalled_Error(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk info": {"", fmt.Errorf("apk: error reading database")},
        })

        pm, _ := NewInstaller("alpine", mock)
        packages, err := pm.ListInstalled(ctx)

        if err == nil {
                t.Fatal("ListInstalled should return an error when apk info fails")
        }
        if packages != nil {
                t.Errorf("expected nil packages on error, got %v", packages)
        }
}

func TestApkInstaller_Search(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk search vim": {"vim-9.0.2050-r0\nvim-doc-9.0.2050-r0\n", nil},
        })

        pm, _ := NewInstaller("alpine", mock)
        results, err := pm.Search(ctx, "vim")

        if err != nil {
                t.Fatalf("Search failed: %v", err)
        }
        expected := []string{"vim-9.0.2050-r0", "vim-doc-9.0.2050-r0"}
        if len(results) != len(expected) {
                t.Fatalf("expected %d results, got %d", len(expected), len(results))
        }
        for i, pkg := range expected {
                if results[i] != pkg {
                        t.Errorf("results[%d] = %q, want %q", i, results[i], pkg)
                }
        }
}

func TestApkInstaller_Search_Error(t *testing.T) {
        ctx := context.Background()

        mock := mockExecFunc(map[string]mockResponse{
                "apk search vim": {"", fmt.Errorf("network unreachable")},
        })

        pm, _ := NewInstaller("alpine", mock)
        results, err := pm.Search(ctx, "vim")

        if err == nil {
                t.Fatal("Search should return an error when apk search fails")
        }
        if results != nil {
                t.Errorf("expected nil results on error, got %v", results)
        }
}

// ---------------------------------------------------------------------------
// ApkInstaller — Install error path test (covers classifyApkError call in Install)
// ---------------------------------------------------------------------------

func TestApkInstaller_Install_PackageNotFound(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"apk add nonexistent": {
			"", fmt.Errorf("could not find package nonexistent"),
		},
	})

	pm, _ := NewInstaller("alpine", mock)
	results, err := pm.Install(ctx, []string{"nonexistent"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install of nonexistent package should fail")
	}
	if results[0].Error != "PACKAGE_NOT_FOUND" {
		t.Errorf("Error = %q, want %q", results[0].Error, "PACKAGE_NOT_FOUND")
	}
}

func TestApkInstaller_Install_PermissionDenied(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"apk add somepkg": {
			"", fmt.Errorf("permission denied: need root"),
		},
	})

	pm, _ := NewInstaller("alpine", mock)
	results, err := pm.Install(ctx, []string{"somepkg"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install with permission denied should fail")
	}
	if results[0].Error != "NO_SUDO" {
		t.Errorf("Error = %q, want %q", results[0].Error, "NO_SUDO")
	}
}

func TestApkInstaller_Install_UnknownError(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"apk add mystery": {
			"", fmt.Errorf("something unexpected happened"),
		},
	})

	pm, _ := NewInstaller("alpine", mock)
	results, err := pm.Install(ctx, []string{"mystery"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install with unknown error should fail")
	}
	if results[0].Error != "UNKNOWN" {
		t.Errorf("Error = %q, want %q", results[0].Error, "UNKNOWN")
	}
}

// ---------------------------------------------------------------------------
// DnfInstaller — Install error path test (covers classifyDnfError call in Install)
// ---------------------------------------------------------------------------

func TestDnfInstaller_Install_PackageNotFound(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"sudo dnf install -y nonexistent": {
			"", fmt.Errorf("No match for argument: nonexistent"),
		},
	})

	pm, _ := NewInstaller("fedora", mock)
	results, err := pm.Install(ctx, []string{"nonexistent"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install of nonexistent package should fail")
	}
	if results[0].Error != "PACKAGE_NOT_FOUND" {
		t.Errorf("Error = %q, want %q", results[0].Error, "PACKAGE_NOT_FOUND")
	}
}

func TestDnfInstaller_Install_AlreadyInstalled(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"sudo dnf install -y somepkg": {
			"", fmt.Errorf("Package somepkg is already installed"),
		},
	})

	pm, _ := NewInstaller("fedora", mock)
	results, err := pm.Install(ctx, []string{"somepkg"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install of already-installed package should report failure")
	}
	if results[0].Error != "ALREADY_INSTALLED" {
		t.Errorf("Error = %q, want %q", results[0].Error, "ALREADY_INSTALLED")
	}
}

func TestDnfInstaller_Install_PermissionDenied(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"sudo dnf install -y somepkg": {
			"", fmt.Errorf("Permission denied: need root"),
		},
	})

	pm, _ := NewInstaller("fedora", mock)
	results, err := pm.Install(ctx, []string{"somepkg"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install with permission denied should fail")
	}
	if results[0].Error != "NO_SUDO" {
		t.Errorf("Error = %q, want %q", results[0].Error, "NO_SUDO")
	}
}

func TestDnfInstaller_Install_UnknownError(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"sudo dnf install -y mystery": {
			"", fmt.Errorf("unexpected repository error"),
		},
	})

	pm, _ := NewInstaller("fedora", mock)
	results, err := pm.Install(ctx, []string{"mystery"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install with unknown error should fail")
	}
	if results[0].Error != "UNKNOWN" {
		t.Errorf("Error = %q, want %q", results[0].Error, "UNKNOWN")
	}
}

// ---------------------------------------------------------------------------
// PacmanInstaller — Install error path test (covers classifyPacmanError call in Install)
// ---------------------------------------------------------------------------

func TestPacmanInstaller_Install_PackageNotFound(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"sudo pacman -S --noconfirm nonexistent": {
			"", fmt.Errorf("target not found: nonexistent"),
		},
	})

	pm, _ := NewInstaller("arch", mock)
	results, err := pm.Install(ctx, []string{"nonexistent"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install of nonexistent package should fail")
	}
	if results[0].Error != "PACKAGE_NOT_FOUND" {
		t.Errorf("Error = %q, want %q", results[0].Error, "PACKAGE_NOT_FOUND")
	}
}

func TestPacmanInstaller_Install_PermissionDenied(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"sudo pacman -S --noconfirm somepkg": {
			"", fmt.Errorf("permission denied: need root"),
		},
	})

	pm, _ := NewInstaller("arch", mock)
	results, err := pm.Install(ctx, []string{"somepkg"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install with permission denied should fail")
	}
	if results[0].Error != "NO_SUDO" {
		t.Errorf("Error = %q, want %q", results[0].Error, "NO_SUDO")
	}
}

func TestPacmanInstaller_Install_TransactionError(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"sudo pacman -S --noconfirm somepkg": {
			"", fmt.Errorf("failed to commit transaction: conflict detected"),
		},
	})

	pm, _ := NewInstaller("arch", mock)
	results, err := pm.Install(ctx, []string{"somepkg"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install with transaction error should fail")
	}
	if results[0].Error != "TRANSACTION_ERROR" {
		t.Errorf("Error = %q, want %q", results[0].Error, "TRANSACTION_ERROR")
	}
}

func TestPacmanInstaller_Install_FileConflict(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"sudo pacman -S --noconfirm somepkg": {
			"", fmt.Errorf("file exists in filesystem: /usr/bin/somepkg"),
		},
	})

	pm, _ := NewInstaller("arch", mock)
	results, err := pm.Install(ctx, []string{"somepkg"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install with file conflict should fail")
	}
	if results[0].Error != "FILE_CONFLICT" {
		t.Errorf("Error = %q, want %q", results[0].Error, "FILE_CONFLICT")
	}
}

func TestPacmanInstaller_Install_UnknownError(t *testing.T) {
	ctx := context.Background()

	mock := mockExecFunc(map[string]mockResponse{
		"sudo pacman -S --noconfirm mystery": {
			"", fmt.Errorf("something unexpected"),
		},
	})

	pm, _ := NewInstaller("arch", mock)
	results, err := pm.Install(ctx, []string{"mystery"})

	if err != nil {
		t.Fatalf("Install should not return error at function level: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("install with unknown error should fail")
	}
	if results[0].Error != "UNKNOWN" {
		t.Errorf("Error = %q, want %q", results[0].Error, "UNKNOWN")
	}
}
