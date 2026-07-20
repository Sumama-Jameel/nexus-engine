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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validProfileYAML is a minimal valid Nexus Profile for reuse in tests.
const validProfileYAML = `name: test-profile
version: "1.0.0"
description: "A test profile"
author: test-author
targets:
  - family: debian
    packages:
      - git
      - curl
env:
  EDITOR: vim
`

// ---------------------------------------------------------------------------
// Parse — read from file
// ---------------------------------------------------------------------------

func TestParse(t *testing.T) {
	t.Run("valid profile from file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "test-profile.yaml")
		if err := os.WriteFile(path, []byte(validProfileYAML), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		profile, err := Parse(path)
		if err != nil {
			t.Fatalf("Parse returned error: %v", err)
		}
		if profile.Name != "test-profile" {
			t.Errorf("expected Name 'test-profile', got %q", profile.Name)
		}
		if profile.Version != "1.0.0" {
			t.Errorf("expected Version '1.0.0', got %q", profile.Version)
		}
		if len(profile.Targets) != 1 {
			t.Fatalf("expected 1 target, got %d", len(profile.Targets))
		}
		if profile.Targets[0].Family != "debian" {
			t.Errorf("expected Family 'debian', got %q", profile.Targets[0].Family)
		}
		if len(profile.Targets[0].Packages) != 2 {
			t.Errorf("expected 2 packages, got %d", len(profile.Targets[0].Packages))
		}
	})

	t.Run("nonexistent file returns error", func(t *testing.T) {
		_, err := Parse("/nonexistent/path/profile.yaml")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("invalid YAML returns error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yaml")
		if err := os.WriteFile(path, []byte("::invalid yaml::\n  [\n"), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := Parse(path)
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})

	t.Run("missing required fields returns error", func(t *testing.T) {
		dir := t.TempDir()

		tests := []struct {
			name    string
			content string
		}{
			{
				name:    "missing name",
				content: "version: \"1.0.0\"\ntargets:\n  - family: debian\n    packages:\n      - git\n",
			},
			{
				name:    "missing version",
				content: "name: test\ntargets:\n  - family: debian\n    packages:\n      - git\n",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				path := filepath.Join(dir, tc.name+".yaml")
				if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
					t.Fatalf("failed to write test file: %v", err)
				}
				_, err := Parse(path)
				if err == nil {
					t.Error("expected error for missing required field")
				}
			})
		}
	})
}

// ---------------------------------------------------------------------------
// ParseBytes — parse from bytes
// ---------------------------------------------------------------------------

func TestParseBytes(t *testing.T) {
	t.Run("valid profile from bytes", func(t *testing.T) {
		profile, err := ParseBytes([]byte(validProfileYAML))
		if err != nil {
			t.Fatalf("ParseBytes returned error: %v", err)
		}
		if profile.Name != "test-profile" {
			t.Errorf("expected Name 'test-profile', got %q", profile.Name)
		}
	})

	t.Run("profile with multiple targets", func(t *testing.T) {
		yaml := `name: multi-target
version: "1.0.0"
targets:
  - family: debian
    packages:
      - git
  - family: arch
    packages:
      - git
  - family: alpine
    packages:
      - git
`
		profile, err := ParseBytes([]byte(yaml))
		if err != nil {
			t.Fatalf("ParseBytes returned error: %v", err)
		}
		if len(profile.Targets) != 3 {
			t.Errorf("expected 3 targets, got %d", len(profile.Targets))
		}
	})

	t.Run("invalid content returns error", func(t *testing.T) {
		_, err := ParseBytes([]byte("not: valid: yaml: ["))
		if err == nil {
			t.Error("expected error for invalid content")
		}
	})

	t.Run("schema rejects unknown family", func(t *testing.T) {
		yaml := `name: bad-family
version: "1.0.0"
targets:
  - family: unknown-os
    packages:
      - git
`
		_, err := ParseBytes([]byte(yaml))
		if err == nil {
			t.Error("expected error for unknown family")
		}
	})

	t.Run("schema rejects empty package list", func(t *testing.T) {
		yaml := `name: empty-packages
version: "1.0.0"
targets:
  - family: debian
    packages: []
`
		_, err := ParseBytes([]byte(yaml))
		if err == nil {
			t.Error("expected error for empty packages")
		}
	})

	t.Run("schema rejects additional properties", func(t *testing.T) {
		yaml := `name: extra-props
version: "1.0.0"
run: "arbitrary_script.sh"
targets:
  - family: debian
    packages:
      - git
`
		_, err := ParseBytes([]byte(yaml))
		if err == nil {
			t.Error("expected error for additional properties (no 'run:' allowed)")
		}
	})

	t.Run("schema rejects invalid name pattern", func(t *testing.T) {
		yaml := `name: "INVALID NAME!"
version: "1.0.0"
targets:
  - family: debian
    packages:
      - git
`
		_, err := ParseBytes([]byte(yaml))
		if err == nil {
			t.Error("expected error for invalid name pattern")
		}
	})

	t.Run("schema rejects invalid version pattern", func(t *testing.T) {
		yaml := `name: test-profile
version: "not-semver"
targets:
  - family: debian
    packages:
      - git
`
		_, err := ParseBytes([]byte(yaml))
		if err == nil {
			t.Error("expected error for invalid version pattern")
		}
	})

	t.Run("schema rejects package with shell metacharacters", func(t *testing.T) {
		yaml := `name: evil-packages
version: "1.0.0"
targets:
  - family: debian
    packages:
      - "git; rm -rf /"
`
		_, err := ParseBytes([]byte(yaml))
		if err == nil {
			t.Error("expected error for package with shell metacharacters")
		}
	})

	t.Run("schema accepts valid package names with dots and dashes", func(t *testing.T) {
		yaml := `name: valid-packages
version: "1.0.0"
targets:
  - family: debian
    packages:
      - python3-pip
      - libssl-dev
      - build-essential
`
		profile, err := ParseBytes([]byte(yaml))
		if err != nil {
			t.Fatalf("expected valid packages, got error: %v", err)
		}
		if len(profile.Targets[0].Packages) != 3 {
			t.Errorf("expected 3 packages, got %d", len(profile.Targets[0].Packages))
		}
	})

	t.Run("profile with extends field", func(t *testing.T) {
		yaml := `name: child-profile
version: "1.0.0"
extends: base-dev
targets:
  - family: debian
    packages:
      - python3-venv
`
		profile, err := ParseBytes([]byte(yaml))
		if err != nil {
			t.Fatalf("ParseBytes returned error: %v", err)
		}
		if profile.Extends != "base-dev" {
			t.Errorf("expected Extends 'base-dev', got %q", profile.Extends)
		}
	})

	t.Run("profile with env vars", func(t *testing.T) {
		profile, err := ParseBytes([]byte(validProfileYAML))
		if err != nil {
			t.Fatalf("ParseBytes returned error: %v", err)
		}
		if profile.Env["EDITOR"] != "vim" {
			t.Errorf("expected EDITOR='vim', got %q", profile.Env["EDITOR"])
		}
	})
}

// ---------------------------------------------------------------------------
// SanitizeProfileName — path traversal prevention
// ---------------------------------------------------------------------------

func TestSanitizeProfileName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid names
		{name: "simple lowercase", input: "base-dev", wantErr: false},
		{name: "starts with digit", input: "0dev", wantErr: false},
		{name: "single char", input: "a", wantErr: false},
		{name: "all digits", input: "123", wantErr: false},
		{name: "hyphenated", input: "my-cool-profile", wantErr: false},

		// Path traversal — security-critical
		{name: "parent traversal", input: "..", wantErr: true, errMsg: "path traversal"},
		{name: "parent traversal with path", input: "../../etc/shadow", wantErr: true, errMsg: "path separators"},
		{name: "slash separator", input: "sub/dir", wantErr: true, errMsg: "path separators"},
		{name: "backslash separator", input: "sub\\dir", wantErr: true, errMsg: "path separators"},
		{name: "dot prefix", input: ".hidden", wantErr: true, errMsg: "cannot start with a dot"},
		{name: "single dot", input: ".", wantErr: true, errMsg: "cannot start with a dot"},

		// Invalid pattern
		{name: "uppercase letters", input: "MyProfile", wantErr: true, errMsg: "must match pattern"},
		{name: "spaces", input: "has spaces", wantErr: true, errMsg: "must match pattern"},
		{name: "underscore", input: "my_profile", wantErr: true, errMsg: "must match pattern"},
		{name: "empty string", input: "", wantErr: true, errMsg: "must match pattern"},
		{name: "starts with hyphen", input: "-profile", wantErr: true, errMsg: "must match pattern"},
		{name: "special chars", input: "profile!@#", wantErr: true, errMsg: "must match pattern"},
		{name: "unicode", input: "профиль", wantErr: true, errMsg: "must match pattern"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := SanitizeProfileName(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q, got nil", tc.input)
				} else if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for input %q: %v", tc.input, err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// IsValidProfileName
// ---------------------------------------------------------------------------

func TestIsValidProfileName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{name: "valid lowercase", input: "base-dev", valid: true},
		{name: "starts with digit", input: "0dev", valid: true},
		{name: "single char", input: "a", valid: true},
		{name: "empty string", input: "", valid: false},
		{name: "uppercase", input: "MyProfile", valid: false},
		{name: "has space", input: "my profile", valid: false},
		{name: "starts with hyphen", input: "-bad", valid: false},
		{name: "has underscore", input: "my_profile", valid: false},
		{name: "has dot", input: "my.profile", valid: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsValidProfileName(tc.input)
			if result != tc.valid {
				t.Errorf("IsValidProfileName(%q) = %v, expected %v", tc.input, result, tc.valid)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Validate — semantic validation (Layer 2)
// ---------------------------------------------------------------------------

func TestValidate(t *testing.T) {
	t.Run("valid profile passes", func(t *testing.T) {
		profile := &NexusProfile{
			Name:    "test",
			Version: "1.0.0",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{"git"}},
			},
		}
		if err := Validate(profile); err != nil {
			t.Errorf("valid profile should pass, got error: %v", err)
		}
	})

	t.Run("missing name fails", func(t *testing.T) {
		profile := &NexusProfile{
			Version: "1.0.0",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{"git"}},
			},
		}
		err := Validate(profile)
		if err == nil {
			t.Error("expected error for missing name")
		}
		if !strings.Contains(err.Error(), "name") {
			t.Errorf("expected error about name, got: %v", err)
		}
	})

	t.Run("missing version fails", func(t *testing.T) {
		profile := &NexusProfile{
			Name: "test",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{"git"}},
			},
		}
		err := Validate(profile)
		if err == nil {
			t.Error("expected error for missing version")
		}
		if !strings.Contains(err.Error(), "version") {
			t.Errorf("expected error about version, got: %v", err)
		}
	})

	t.Run("no targets and no extends fails", func(t *testing.T) {
		profile := &NexusProfile{
			Name:    "test",
			Version: "1.0.0",
		}
		err := Validate(profile)
		if err == nil {
			t.Error("expected error for no targets and no extends")
		}
		if !strings.Contains(err.Error(), "target") {
			t.Errorf("expected error about targets, got: %v", err)
		}
	})

	t.Run("extends without targets is valid", func(t *testing.T) {
		profile := &NexusProfile{
			Name:    "child",
			Version: "1.0.0",
			Extends: "parent",
		}
		if err := Validate(profile); err != nil {
			t.Errorf("extends without targets should be valid, got: %v", err)
		}
	})

	t.Run("unknown family fails", func(t *testing.T) {
		profile := &NexusProfile{
			Name:    "test",
			Version: "1.0.0",
			Targets: []TargetConfig{
				{Family: "beos", Packages: []string{"git"}},
			},
		}
		err := Validate(profile)
		if err == nil {
			t.Error("expected error for unknown family")
		}
		if !strings.Contains(err.Error(), "not in the allowed list") {
			t.Errorf("expected error about allowed list, got: %v", err)
		}
	})

	t.Run("empty packages list fails", func(t *testing.T) {
		profile := &NexusProfile{
			Name:    "test",
			Version: "1.0.0",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{}},
			},
		}
		err := Validate(profile)
		if err == nil {
			t.Error("expected error for empty packages list")
		}
	})

	t.Run("empty package name fails", func(t *testing.T) {
		profile := &NexusProfile{
			Name:    "test",
			Version: "1.0.0",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{""}},
			},
		}
		err := Validate(profile)
		if err == nil {
			t.Error("expected error for empty package name")
		}
		if !strings.Contains(err.Error(), "cannot be empty") {
			t.Errorf("expected error about empty package, got: %v", err)
		}
	})

	t.Run("self-extends fails", func(t *testing.T) {
		profile := &NexusProfile{
			Name:    "test",
			Version: "1.0.0",
			Extends: "test",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{"git"}},
			},
		}
		err := Validate(profile)
		if err == nil {
			t.Error("expected error for self-extends")
		}
		if !strings.Contains(err.Error(), "cannot extend itself") {
			t.Errorf("expected self-extends error, got: %v", err)
		}
	})

	t.Run("all valid families pass", func(t *testing.T) {
		for family := range AllowedPackageFamilies {
			profile := &NexusProfile{
				Name:    "test-" + family,
				Version: "1.0.0",
				Targets: []TargetConfig{
					{Family: family, Packages: []string{"git"}},
				},
			}
			if err := Validate(profile); err != nil {
				t.Errorf("family %q should be valid, got error: %v", family, err)
			}
		}
	})

	t.Run("empty family string fails", func(t *testing.T) {
		profile := &NexusProfile{
			Name:    "test",
			Version: "1.0.0",
			Targets: []TargetConfig{
				{Family: "", Packages: []string{"git"}},
			},
		}
		err := Validate(profile)
		if err == nil {
			t.Error("expected error for empty family")
		}
	})
}

// ---------------------------------------------------------------------------
// Two-layer validation: Schema layer + Go semantic layer
// ---------------------------------------------------------------------------

func TestTwoLayerValidation(t *testing.T) {
	t.Run("schema layer catches structural violations", func(t *testing.T) {
		// This YAML has valid semantic structure but violates schema constraints
		yaml := `name: "INVALID"
version: "1.0.0"
targets:
  - family: debian
    packages:
      - git
`
		_, err := ParseBytes([]byte(yaml))
		if err == nil {
			t.Error("schema should reject uppercase name")
		}
		if !strings.Contains(err.Error(), "SCHEMA VALIDATION FAILED") {
			t.Errorf("expected schema validation failure, got: %v", err)
		}
	})

	t.Run("semantic layer catches business rule violations", func(t *testing.T) {
		// This YAML passes schema but fails semantic validation
		yaml := `name: test-profile
version: "1.0.0"
extends: test-profile
targets:
  - family: debian
    packages:
      - git
`
		_, err := ParseBytes([]byte(yaml))
		if err == nil {
			t.Error("semantic validation should reject self-extends")
		}
		// Self-extends is caught by Validate() which runs after schema validation
		if !strings.Contains(err.Error(), "cannot extend itself") {
			t.Errorf("expected self-extends error, got: %v", err)
		}
	})

	t.Run("both layers must pass", func(t *testing.T) {
		yaml := `name: valid-profile
version: "1.0.0"
targets:
  - family: debian
    packages:
      - git
      - curl
`
		profile, err := ParseBytes([]byte(yaml))
		if err != nil {
			t.Fatalf("both layers should pass for valid profile, got: %v", err)
		}
		if profile.Name != "valid-profile" {
			t.Errorf("expected Name 'valid-profile', got %q", profile.Name)
		}
	})
}

// ---------------------------------------------------------------------------
// ResolveExtends — inheritance with cycle detection and depth limit
// ---------------------------------------------------------------------------

// mockLoader is a simple in-memory ProfileLoader for testing.
type mockLoader struct {
	profiles map[string]*NexusProfile
}

func (m *mockLoader) LoadProfile(name string) (*NexusProfile, error) {
	p, ok := m.profiles[name]
	if !ok {
		return nil, fmt.Errorf("profile '%s' not found", name)
	}
	return p, nil
}

func TestResolveExtends(t *testing.T) {
	t.Run("no extends returns profile unchanged", func(t *testing.T) {
		profile := &NexusProfile{
			Name:    "base",
			Version: "1.0.0",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{"git"}},
			},
		}
		result, err := ResolveExtends(profile, &mockLoader{}, make(map[string]bool), 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Name != "base" {
			t.Errorf("expected Name 'base', got %q", result.Name)
		}
	})

	t.Run("single level extends merges packages", func(t *testing.T) {
		parent := &NexusProfile{
			Name:    "parent",
			Version: "1.0.0",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{"git", "curl"}},
			},
			Env: map[string]string{"EDITOR": "vim"},
		}
		child := &NexusProfile{
			Name:    "child",
			Version: "2.0.0",
			Extends: "parent",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{"python3"}},
			},
			Env: map[string]string{"LANG": "en_US"},
		}

		loader := &mockLoader{profiles: map[string]*NexusProfile{"parent": parent}}
		result, err := ResolveExtends(child, loader, make(map[string]bool), 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Child metadata takes precedence
		if result.Name != "child" {
			t.Errorf("expected Name 'child', got %q", result.Name)
		}
		if result.Version != "2.0.0" {
			t.Errorf("expected Version '2.0.0', got %q", result.Version)
		}

		// Packages should be merged (parent + child, deduplicated)
		totalPkgs := 0
		for _, t := range result.Targets {
			totalPkgs += len(t.Packages)
		}
		if totalPkgs < 3 { // git, curl, python3
			t.Errorf("expected at least 3 merged packages, got %d", totalPkgs)
		}

		// Env should be merged
		if result.Env["EDITOR"] != "vim" {
			t.Error("expected parent env EDITOR=vim to be inherited")
		}
		if result.Env["LANG"] != "en_US" {
			t.Error("expected child env LANG=en_US")
		}
	})

	t.Run("multi-level extends resolves recursively", func(t *testing.T) {
		grandparent := &NexusProfile{
			Name:    "grandparent",
			Version: "1.0.0",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{"git"}},
			},
			Env: map[string]string{"A": "1"},
		}
		parent := &NexusProfile{
			Name:    "parent",
			Version: "1.0.0",
			Extends: "grandparent",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{"curl"}},
			},
			Env: map[string]string{"B": "2"},
		}
		child := &NexusProfile{
			Name:    "child",
			Version: "1.0.0",
			Extends: "parent",
			Targets: []TargetConfig{
				{Family: "debian", Packages: []string{"wget"}},
			},
			Env: map[string]string{"C": "3"},
		}

		loader := &mockLoader{
			profiles: map[string]*NexusProfile{
				"grandparent": grandparent,
				"parent":      parent,
			},
		}
		result, err := ResolveExtends(child, loader, make(map[string]bool), 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// All env vars should be present
		if result.Env["A"] != "1" {
			t.Error("expected grandparent env A=1")
		}
		if result.Env["B"] != "2" {
			t.Error("expected parent env B=2")
		}
		if result.Env["C"] != "3" {
			t.Error("expected child env C=3")
		}
	})

	t.Run("cycle detection prevents infinite loop", func(t *testing.T) {
		a := &NexusProfile{
			Name:    "a",
			Version: "1.0.0",
			Extends: "b",
			Targets: []TargetConfig{{Family: "debian", Packages: []string{"git"}}},
		}
		b := &NexusProfile{
			Name:    "b",
			Version: "1.0.0",
			Extends: "a",
			Targets: []TargetConfig{{Family: "debian", Packages: []string{"curl"}}},
		}

		loader := &mockLoader{profiles: map[string]*NexusProfile{"a": a, "b": b}}
		_, err := ResolveExtends(a, loader, make(map[string]bool), 0)
		if err == nil {
			t.Error("expected error for circular extends")
		}
		if !strings.Contains(err.Error(), "circular") {
			t.Errorf("expected circular extends error, got: %v", err)
		}
	})

	t.Run("depth limit exceeded", func(t *testing.T) {
		loader := &mockLoader{profiles: map[string]*NexusProfile{}}

		// Create a chain longer than MaxExtendsDepth
		profiles := make(map[string]*NexusProfile)
		for i := 0; i <= MaxExtendsDepth+1; i++ {
			name := fmt.Sprintf("level-%d", i)
			p := &NexusProfile{
				Name:    name,
				Version: "1.0.0",
				Targets: []TargetConfig{{Family: "debian", Packages: []string{"git"}}},
			}
			if i < MaxExtendsDepth+1 {
				p.Extends = fmt.Sprintf("level-%d", i+1)
			}
			profiles[name] = p
		}
		loader.profiles = profiles

		_, err := ResolveExtends(profiles["level-0"], loader, make(map[string]bool), 0)
		if err == nil {
			t.Error("expected error for depth limit exceeded")
		}
		if !strings.Contains(err.Error(), "maximum depth") {
			t.Errorf("expected depth limit error, got: %v", err)
		}
	})

	t.Run("child env overrides parent", func(t *testing.T) {
		parent := &NexusProfile{
			Name:    "parent",
			Version: "1.0.0",
			Targets: []TargetConfig{{Family: "debian", Packages: []string{"git"}}},
			Env:     map[string]string{"EDITOR": "vim", "SHELL": "bash"},
		}
		child := &NexusProfile{
			Name:    "child",
			Version: "1.0.0",
			Extends: "parent",
			Targets: []TargetConfig{{Family: "debian", Packages: []string{"curl"}}},
			Env:     map[string]string{"EDITOR": "nano"},
		}

		loader := &mockLoader{profiles: map[string]*NexusProfile{"parent": parent}}
		result, err := ResolveExtends(child, loader, make(map[string]bool), 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Env["EDITOR"] != "nano" {
			t.Errorf("child should override parent env, got EDITOR=%q", result.Env["EDITOR"])
		}
		if result.Env["SHELL"] != "bash" {
			t.Errorf("parent env should be preserved when child doesn't override, got SHELL=%q", result.Env["SHELL"])
		}
	})

	t.Run("parent not found returns error", func(t *testing.T) {
		child := &NexusProfile{
			Name:    "child",
			Version: "1.0.0",
			Extends: "nonexistent",
			Targets: []TargetConfig{{Family: "debian", Packages: []string{"git"}}},
		}

		loader := &mockLoader{profiles: map[string]*NexusProfile{}}
		_, err := ResolveExtends(child, loader, make(map[string]bool), 0)
		if err == nil {
			t.Error("expected error for missing parent profile")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' error, got: %v", err)
		}
	})

	t.Run("Extends field cleared after resolution", func(t *testing.T) {
		parent := &NexusProfile{
			Name:    "parent",
			Version: "1.0.0",
			Targets: []TargetConfig{{Family: "debian", Packages: []string{"git"}}},
		}
		child := &NexusProfile{
			Name:    "child",
			Version: "1.0.0",
			Extends: "parent",
			Targets: []TargetConfig{{Family: "debian", Packages: []string{"curl"}}},
		}

		loader := &mockLoader{profiles: map[string]*NexusProfile{"parent": parent}}
		result, err := ResolveExtends(child, loader, make(map[string]bool), 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Extends != "" {
			t.Errorf("Extends should be cleared after resolution, got %q", result.Extends)
		}
	})
}

// ---------------------------------------------------------------------------
// FormatProfileYAML
// ---------------------------------------------------------------------------

func TestFormatProfileYAML(t *testing.T) {
	profile := &NexusProfile{
		Name:        "test",
		Version:     "1.0.0",
		Description: "A test profile",
		Author:      "tester",
		Targets: []TargetConfig{
			{Family: "debian", Packages: []string{"git", "curl"}},
		},
		Env: map[string]string{"EDITOR": "vim"},
	}

	yaml, err := FormatProfileYAML(profile)
	if err != nil {
		t.Fatalf("FormatProfileYAML returned error: %v", err)
	}
	if !strings.Contains(yaml, "name: test") {
		t.Error("YAML output should contain profile name")
	}
	if !strings.Contains(yaml, "family: debian") {
		t.Error("YAML output should contain target family")
	}
}

// ---------------------------------------------------------------------------
// AllowedPackageFamilies
// ---------------------------------------------------------------------------

func TestAllowedPackageFamilies(t *testing.T) {
	expected := []string{"debian", "arch", "fedora", "alpine", "ubuntu", "linux"}
	for _, family := range expected {
		if !AllowedPackageFamilies[family] {
			t.Errorf("family %q should be in AllowedPackageFamilies", family)
		}
	}
}

// ---------------------------------------------------------------------------
// MaxExtendsDepth
// ---------------------------------------------------------------------------

func TestMaxExtendsDepth(t *testing.T) {
	if MaxExtendsDepth != 5 {
		t.Errorf("MaxExtendsDepth should be 5, got %d", MaxExtendsDepth)
	}
}
