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

package manifest

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/xeipuuv/gojsonschema"
)

//go:embed schemas/nexus-profile.schema.json
var schemaFS embed.FS

// NexusProfile represents a validated Nexus configuration manifest.
// Per the Nexus Protocol: "We do not write installation scripts; we write a
// schema." and "We will never allow a run: arbitrary_script.sh in our manifests."
//
// A profile declares which packages to install, organized by OS family.
// The engine translates these declarations into safe, auditable package
// manager commands via the Orchestrator. Profiles never contain executable
// code — only declarative package lists.
//
// The Extends field enables profile inheritance: a child profile can
// inherit and augment the packages of a parent profile. Resolution is
// performed by ResolveExtends with cycle detection and depth limiting.
type NexusProfile struct {
	Name            string            `yaml:"name"`
	Version         string            `yaml:"version"`
	Description     string            `yaml:"description"`
	Author          string            `yaml:"author"`
	Extends         string            `yaml:"extends,omitempty"`
	Targets         []TargetConfig    `yaml:"targets"`
	Env             map[string]string `yaml:"env"`
	Dotfiles        *DotfilesSpec     `yaml:"dotfiles,omitempty"`
	SuggestedDistro string            `yaml:"suggested_distro,omitempty"`
	WSL2Available   bool              `yaml:"wsl_available,omitempty"`
}

// DotfilesSpec declares the V7 dotfile-management intent for a profile.
// Per the V7 plan: a profile declares WHAT dotfiles to manage, not HOW.
// The engine translates this into calls to the dotfiles bounded context
// (bind, apply, add).
//
// All fields are optional:
//   - Source: when set, the engine binds this repo on profile apply.
//   - ApplyOnInit: when true, applies dotfiles during 'nexus init'.
//   - ManagedPaths: when set, tracks each path with 'chezmoi add'.
//   - SensitivePaths: documentation only; --force is still required at runtime.
//   - SyncOnApply: when true (V8), 'nexus profile apply' triggers a full
//     pull + apply + push sync after the bind step. Lets profile switching
//     be a sync point.
type DotfilesSpec struct {
	Source         string   `yaml:"source,omitempty"`
	ApplyOnInit    bool     `yaml:"apply_on_init,omitempty"`
	ManagedPaths   []string `yaml:"managed_paths,omitempty"`
	SensitivePaths []string `yaml:"sensitive_paths,omitempty"`
	SyncOnApply    bool     `yaml:"sync_on_apply,omitempty"`
}

// TargetConfig defines a set of packages for a specific OS family.
// The Family field must be one of the keys in AllowedPackageFamilies.
// The Packages field contains ONLY package names — no scripts, URLs, or
// shell commands are permitted. This is enforced by the JSON Schema
// (pattern restrictions) and the semantic validation layer.
type TargetConfig struct {
	Family   string   `yaml:"family"`   // e.g., "debian", "arch", "fedora", "alpine"
	Packages []string `yaml:"packages"` // ONLY package names — no scripts allowed
}

// AllowedPackageFamilies defines the only permitted package families for
// profile targets. Per the plan: "only approved package managers (apt, pacman,
// npm, pip) can be used."
//
// Security rationale: Package families map directly to package manager commands
// (e.g., "debian" → apt-get, "arch" → pacman). An unregistered family would
// require the engine to execute an unknown command, which is a command injection
// vector. By restricting families to this explicit whitelist, the engine can
// safely translate each family to its known, safe command without risk of
// executing arbitrary programs.
var AllowedPackageFamilies = map[string]bool{
	"debian": true, // apt-get
	"arch":   true, // pacman
	"fedora": true, // dnf
	"alpine": true, // apk
	"ubuntu": true, // apt-get
	"linux":  true, // generic (all families)
}

// MaxExtendsDepth is the maximum allowed depth for profile extends chains.
// This limit prevents degenerate chains that could cause excessive recursion,
// stack overflow, or resource exhaustion during profile resolution.
// The value 5 allows reasonable inheritance hierarchies while bounding
// computational cost.
const MaxExtendsDepth = 5

// Parse reads a Nexus Profile YAML file from disk and validates it.
// Validation is a two-layer security gate:
//  1. JSON Schema validation (structural + pattern constraints via embedded schema)
//  2. Go-level semantic validation (family mapping, non-empty packages, no self-extends)
//
// Both layers must pass for the profile to be accepted. A profile that fails
// either layer is rejected — there is no "partial" acceptance.
//
// Callers should also call SanitizeProfileName on the filename before calling
// Parse to prevent path traversal attacks.
func Parse(path string) (*NexusProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest '%s': %w", path, err)
	}

	return ParseBytes(data)
}

// ParseBytes parses and validates a Nexus Profile from raw YAML bytes.
// This is the core parsing function used by Parse, the ProfileStore, and
// the remote fetcher — all of which already have the content in memory.
//
// The two-layer validation gate is applied in order:
//  1. JSON Schema validation rejects structural violations
//  2. Semantic validation rejects business-rule violations
//
// If either layer fails, the function returns an error and the profile
// is not usable. The error message distinguishes between schema failures
// (prefixed "SCHEMA VALIDATION FAILED") and semantic failures (prefixed
// "VALIDATION").
func ParseBytes(data []byte) (*NexusProfile, error) {
	// Layer 1: JSON Schema validation
	if err := validateAgainstSchema(data); err != nil {
		return nil, fmt.Errorf("SCHEMA VALIDATION FAILED: %w", err)
	}

	// Layer 2: Go-level semantic validation
	var profile NexusProfile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse YAML manifest: %w", err)
	}

	if err := Validate(&profile); err != nil {
		return nil, err
	}

	return &profile, nil
}

// validateAgainstSchema converts YAML to JSON and validates against the
// embedded JSON Schema. This enforces ALL constraints defined in the schema:
// pattern restrictions on name/version/packages, additionalProperties: false,
// minItems on arrays, maxLength on strings, etc.
//
// Per the Nexus Protocol: "The Engine operates on a Contract System."
// The JSON Schema IS the contract. No profile may violate it.
func validateAgainstSchema(yamlData []byte) error {
	// Convert YAML to JSON for schema validation
	var raw interface{}
	if err := yaml.Unmarshal(yamlData, &raw); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	jsonData, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to convert YAML to JSON: %w", err)
	}

	// Load the embedded schema
	schemaBytes, err := schemaFS.ReadFile("schemas/nexus-profile.schema.json")
	if err != nil {
		return fmt.Errorf("failed to load embedded schema: %w", err)
	}

	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
	documentLoader := gojsonschema.NewBytesLoader(jsonData)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("schema validation engine error: %w", err)
	}

	if !result.Valid() {
		var errs []string
		for _, desc := range result.Errors() {
			errs = append(errs, desc.String())
		}
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}

	return nil
}

// Validate ensures the profile conforms to the Nexus semantic contract.
// This is Layer 2 of the two-gate validation system — it catches errors
// that JSON Schema cannot express:
//   - Family must map to a known package manager (checked against AllowedPackageFamilies)
//   - Package names cannot be empty (prevents silent no-op installs)
//   - A profile cannot extend itself (prevents trivial cycles)
//   - At least one target or an extends field is required (prevents empty profiles)
//
// Both Validate and the JSON Schema validation must pass for a profile to
// be accepted by Parse or ParseBytes.
func Validate(profile *NexusProfile) error {
	if profile.Name == "" {
		return fmt.Errorf("VALIDATION: profile 'name' is required")
	}

	if profile.Version == "" {
		return fmt.Errorf("VALIDATION: profile 'version' is required")
	}

	if len(profile.Targets) == 0 && profile.Extends == "" {
		return fmt.Errorf("VALIDATION: at least one 'target' is required (or specify 'extends')")
	}

	for i, target := range profile.Targets {
		if target.Family == "" {
			return fmt.Errorf("VALIDATION: targets[%d].family is required", i)
		}
		if !AllowedPackageFamilies[target.Family] {
			return fmt.Errorf("VALIDATION: targets[%d].family '%s' is not in the allowed list: %v",
				i, target.Family, allowedFamiliesList())
		}
		if len(target.Packages) == 0 {
			return fmt.Errorf("VALIDATION: targets[%d].packages must contain at least one package", i)
		}
		for j, pkg := range target.Packages {
			if pkg == "" {
				return fmt.Errorf("VALIDATION: targets[%d].packages[%d] cannot be empty", i, j)
			}
		}
	}

	// Validate extends field if present
	if profile.Extends != "" {
		if profile.Extends == profile.Name {
			return fmt.Errorf("VALIDATION: profile cannot extend itself")
		}
	}

	// Validate dotfiles section if present.
	// The JSON Schema handles structural validation; this is the
	// semantic layer that catches what schema cannot express.
	if profile.Dotfiles != nil {
		for i, p := range profile.Dotfiles.ManagedPaths {
			if p == "" {
				return fmt.Errorf("VALIDATION: dotfiles.managed_paths[%d] cannot be empty", i)
			}
			if !strings.HasPrefix(p, "/") {
				return fmt.Errorf("VALIDATION: dotfiles.managed_paths[%d] must be an absolute path (got %q)", i, p)
			}
		}
		if profile.Dotfiles.Source != "" && !strings.HasPrefix(profile.Dotfiles.Source, "https://") {
			return fmt.Errorf("VALIDATION: dotfiles.source must be an HTTPS URL (got %q)", profile.Dotfiles.Source)
		}
	}

	return nil
}

// ResolveExtends recursively resolves the extends chain and merges profiles.
// The child profile overlays the parent: packages are additive (merged within
// same family), env vars from child override parent, and metadata from child
// takes precedence.
//
// Safety mechanisms:
//   - Cycle detection: a visited set tracks all profiles in the chain. If a
//     profile appears twice, the chain is circular and is REJECTED immediately.
//   - Depth limit: MaxExtendsDepth (5) prevents degenerate chains that could
//     cause stack overflow or resource exhaustion.
//
// Supply chain security: every profile in the chain is individually loaded
// through the ProfileLoader (which validates and integrity-checks each one)
// before merging. A compromised parent cannot bypass validation.
func ResolveExtends(profile *NexusProfile, loader ProfileLoader, visited map[string]bool, depth int) (*NexusProfile, error) {
	if profile.Extends == "" {
		return profile, nil
	}

	if depth >= MaxExtendsDepth {
		return nil, fmt.Errorf("extends chain exceeds maximum depth of %d — possible degenerate chain", MaxExtendsDepth)
	}

	// Cycle detection
	if visited[profile.Name] {
		return nil, fmt.Errorf("circular extends detected: profile '%s' appears twice in chain", profile.Name)
	}
	visited[profile.Name] = true

	// Load the parent profile
	parent, err := loader.LoadProfile(profile.Extends)
	if err != nil {
		return nil, fmt.Errorf("failed to load parent profile '%s': %w", profile.Extends, err)
	}

	// Recursively resolve the parent's extends first
	resolvedParent, err := ResolveExtends(parent, loader, visited, depth+1)
	if err != nil {
		return nil, err
	}

	// Merge: parent as base, child overlays
	return mergeProfiles(resolvedParent, profile), nil
}

// mergeProfiles combines a parent and child profile. The child overlays:
// - Packages: additive merge within same family (deduplicated)
// - Env: child wins for same key, parent keys preserved otherwise
// - Metadata (name, version, description, author): child wins
func mergeProfiles(parent, child *NexusProfile) *NexusProfile {
	merged := &NexusProfile{
		Name:        child.Name,
		Version:     child.Version,
		Description: child.Description,
		Author:      child.Author,
		Extends:     "", // Resolved — no longer needed
		Targets:     []TargetConfig{},
		Env:         make(map[string]string),
	}

	// Copy parent env
	for k, v := range parent.Env {
		merged.Env[k] = v
	}
	// Child env overrides
	for k, v := range child.Env {
		merged.Env[k] = v
	}

	// Build package map: family -> deduplicated package set
	pkgMap := make(map[string]map[string]bool)

	// Add parent packages
	for _, target := range parent.Targets {
		if pkgMap[target.Family] == nil {
			pkgMap[target.Family] = make(map[string]bool)
		}
		for _, pkg := range target.Packages {
			pkgMap[target.Family][pkg] = true
		}
	}

	// Add child packages (dedup handled by map)
	for _, target := range child.Targets {
		if pkgMap[target.Family] == nil {
			pkgMap[target.Family] = make(map[string]bool)
		}
		for _, pkg := range target.Packages {
			pkgMap[target.Family][pkg] = true
		}
	}

	// Convert map back to TargetConfig slice
	for family, pkgs := range pkgMap {
		var pkgList []string
		for pkg := range pkgs {
			pkgList = append(pkgList, pkg)
		}
		merged.Targets = append(merged.Targets, TargetConfig{
			Family:   family,
			Packages: pkgList,
		})
	}

	// If child has no targets and no extends, inherit parent targets
	if len(child.Targets) == 0 && child.Extends == "" && len(merged.Targets) == 0 {
		merged.Targets = parent.Targets
	}

	return merged
}

// ProfileLoader is the interface for loading profiles by name. It enables
// the extends resolution system to work with any profile source (local store,
// embedded defaults, remote fetch) without coupling to a specific backend.
// The ProfileStore type implements this interface.
type ProfileLoader interface {
	LoadProfile(name string) (*NexusProfile, error)
}

// FormatProfileYAML serializes a NexusProfile back to a YAML string with
// 2-space indentation. Used by `nexus profile create` to write new profiles
// to disk. The output is human-readable and suitable for manual editing.
func FormatProfileYAML(profile *NexusProfile) (string, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(profile); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func allowedFamiliesList() []string {
	families := make([]string, 0, len(AllowedPackageFamilies))
	for f := range AllowedPackageFamilies {
		families = append(families, f)
	}
	sort.Strings(families)
	return families
}
