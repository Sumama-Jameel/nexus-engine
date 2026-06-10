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
        "time"

        "github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// PackageResult captures the outcome of a single package operation.
// Every installation, removal, and update returns this structure —
// never raw strings, never just an error code. This is the contract.
type PackageResult struct {
        // Package is the name of the package this result pertains to.
        Package    string        `json:"package"`
        // Action is the operation performed: "install", "remove", or "update".
        Action     string        `json:"action"`
        // Error contains a human-readable or classified error message when
        // Success is false. Empty when the operation succeeded.
        Error      string        `json:"error,omitempty"`
        // SkipReason explains why the operation was skipped, e.g.,
        // "already installed".
        SkipReason string        `json:"skip_reason,omitempty"`
        // Duration is the wall-clock time taken for the package operation.
        Duration   time.Duration `json:"duration_ms"`
        // Success indicates whether the package operation completed without error.
        Success    bool          `json:"success"`
        // Verified indicates whether post-install verification confirmed the
        // package is functional. Only set after the Verify step; false until then.
        Verified   bool          `json:"verified"`
        // Skipped indicates the operation was skipped because the package was
        // already in the desired state (e.g., already installed).
        Skipped    bool          `json:"skipped"`
}

// PackageManager is the contract every package manager must implement.
// Per DDD: this is the bounded context's interface. The Orchestrator
// depends on this interface, never on a concrete implementation.
// Per Zero-Trust: every method takes context.Context for timeout enforcement.
type PackageManager interface {
        // RefreshIndex updates the local package index/cache.
        // Per the Nexus Protocol: without a fresh index, apt-get may fail
        // with "Unable to locate package" on fresh systems.
        // This is the equivalent of `apt-get update`, `pacman -Sy`, etc.
        RefreshIndex(ctx context.Context) error

        // Install installs the given packages. Returns one PackageResult per package.
        Install(ctx context.Context, packages []string) ([]PackageResult, error)
        // Remove removes the given packages.
        Remove(ctx context.Context, packages []string) ([]PackageResult, error)
        // Update updates the given packages. If empty, updates all managed packages.
        Update(ctx context.Context, packages []string) ([]PackageResult, error)
        // IsInstalled checks if a single package is installed on the system.
        IsInstalled(ctx context.Context, pkg string) bool
        // ListInstalled returns all packages installed on the system via this manager.
        ListInstalled(ctx context.Context) ([]string, error)
        // Search searches for packages matching the query.
        Search(ctx context.Context, query string) ([]string, error)
        // Name returns the package manager's identifier (e.g., "apt", "pacman").
        Name() string
}

// ExecFunc is the type signature for the SanitizeAndExecute function.
// Per Zero-Trust: no installer can bypass the security gate.
// This is dependency injection — the installer receives the execution
// function rather than calling exec.Command directly.
type ExecFunc func(ctx context.Context, command string, args ...string) (string, error)

// NewInstaller is the factory function. It returns the correct PackageManager
// implementation based on the detected OS family.
// Per the Open/Closed Principle: adding a new package manager means adding
// a new case here and a new file — zero modifications to existing code.
func NewInstaller(family string, execFn ExecFunc) (PackageManager, error) {
        if execFn == nil {
                return nil, fmt.Errorf("SECURITY: ExecFunc must not be nil — Zero-Trust boundary")
        }

        switch family {
        case "debian", "ubuntu":
                return &AptInstaller{execFn: execFn}, nil
        case "arch":
                return &PacmanInstaller{execFn: execFn}, nil
        case "fedora":
                return &DnfInstaller{execFn: execFn}, nil
        case "alpine":
                return &ApkInstaller{execFn: execFn}, nil
        default:
                return nil, fmt.Errorf("unsupported package family: '%s'", family)
        }
}

// Priority constants define the installation ordering for the Orchestrator.
// Per the BASIC_QNA doc: "You need to manually design the logic of how
// the Go engine decides what to install first."
//
// The rule: Foundation before Languages before Tools.
// If ca-certificates fails, we abort — nothing works without TLS trust.
// If python3 fails, python3-pip will fail — we abort the tools group.
const (
        // PriorityFoundation is the highest priority group (1). Foundation packages
        // provide the base layer required by all other packages: TLS trust roots,
        // GPG key management, and C/C++ build toolchains. Examples: ca-certificates,
        // gnupg, build-essential, base-devel, build-base, gcc.
        PriorityFoundation = 1

        // PriorityLanguage is the middle priority group (2). Language runtimes
        // and compilers depend on the foundation layer. Examples: python3,
        // openjdk-21-jdk, nodejs, npm.
        PriorityLanguage = 2

        // PriorityTool is the lowest priority group (3). End-user tools and
        // utilities depend on both the foundation and language layers.
        // Examples: git, curl, vim, htop, tmux.
        PriorityTool = 3
)

// ClassifyPriority determines the installation priority of a package.
// This is the Orchestrator's dependency-aware ordering heuristic.
func ClassifyPriority(pkg string) int {
        foundations := map[string]bool{
                "ca-certificates": true, "gnupg": true, "gnupg2": true,
                "build-essential": true, "base-devel": true, "build-base": true,
                "gcc": true,
        }
        languages := map[string]bool{
                "python3": true, "python": true, "python3-pip": true, "py3-pip": true,
                "python-pip": true, "pip": true,
                "openjdk-21-jdk": true, "jdk-openjdk": true,
                "java-21-openjdk-devel": true, "openjdk21": true,
                "nodejs": true, "npm": true,
        }

        if foundations[pkg] {
                return PriorityFoundation
        }
        if languages[pkg] {
                return PriorityLanguage
        }
        return PriorityTool
}

// BuildDependencyMap constructs a mapping from foundation packages to the
// language packages that depend on them. This is used by the remove command
// to warn when removing a package that other managed packages depend on.
// The dependency heuristic is based on priority classification: every language
// package depends on every foundation package (since language runtimes require
// a foundation layer to function).
func BuildDependencyMap(managed map[string]engine.PackageState) map[string][]string {
        depMap := make(map[string][]string)
        var foundations, languages []string
        for pkg := range managed {
                p := ClassifyPriority(pkg)
                switch p {
                case PriorityFoundation:
                        foundations = append(foundations, pkg)
                case PriorityLanguage:
                        languages = append(languages, pkg)
                }
        }
        for _, lang := range languages {
                for _, found := range foundations {
                        depMap[found] = append(depMap[found], lang)
                }
        }
        return depMap
}
