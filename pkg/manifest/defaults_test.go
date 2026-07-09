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
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// BundledDefaults
// ---------------------------------------------------------------------------

func TestBundledDefaults_ReturnsNonNilMap(t *testing.T) {
	defaults := BundledDefaults()
	if defaults == nil {
		t.Fatal("BundledDefaults() should return a non-nil map")
	}
}

func TestBundledDefaults_ReturnsNonEmptyMap(t *testing.T) {
	defaults := BundledDefaults()
	if len(defaults) == 0 {
		t.Fatal("BundledDefaults() should return a non-empty map")
	}
}

func TestBundledDefaults_ContainsBaseDev(t *testing.T) {
	defaults := BundledDefaults()
	content, exists := defaults["base-dev"]
	if !exists {
		t.Fatal("BundledDefaults() should contain 'base-dev' profile")
	}
	if content == "" {
		t.Error("'base-dev' profile content should not be empty")
	}
}

func TestBundledDefaults_ContainsDataScience(t *testing.T) {
	defaults := BundledDefaults()
	content, exists := defaults["data-science"]
	if !exists {
		t.Fatal("BundledDefaults() should contain 'data-science' profile")
	}
	if content == "" {
		t.Error("'data-science' profile content should not be empty")
	}
}

func TestBundledDefaults_EachProfileIsValidYAML(t *testing.T) {
	defaults := BundledDefaults()
	for name, content := range defaults {
		t.Run(name, func(t *testing.T) {
			var profile NexusProfile
			if err := yaml.Unmarshal([]byte(content), &profile); err != nil {
				t.Errorf("profile %q is not valid YAML: %v", name, err)
			}
		})
	}
}

func TestBundledDefaults_EachProfilePassesFullValidation(t *testing.T) {
	defaults := BundledDefaults()
	for name, content := range defaults {
		t.Run(name, func(t *testing.T) {
			_, err := ParseBytes([]byte(content))
			if err != nil {
				t.Errorf("profile %q should pass full ParseBytes validation: %v", name, err)
			}
		})
	}
}

func TestBundledDefaults_EachProfileHasRequiredFields(t *testing.T) {
	defaults := BundledDefaults()
	for name, content := range defaults {
		t.Run(name, func(t *testing.T) {
			var profile NexusProfile
			if err := yaml.Unmarshal([]byte(content), &profile); err != nil {
				t.Fatalf("failed to parse profile %q: %v", name, err)
			}

			// name is required
			if profile.Name == "" {
				t.Errorf("profile %q: 'name' field is required", name)
			}
			// version is required
			if profile.Version == "" {
				t.Errorf("profile %q: 'version' field is required", name)
			}
			// targets is required (unless extends is set)
			if len(profile.Targets) == 0 && profile.Extends == "" {
				t.Errorf("profile %q: must have at least one 'target' or specify 'extends'", name)
			}
		})
	}
}

func TestBundledDefaults_BaseDevProfileFields(t *testing.T) {
	defaults := BundledDefaults()
	content, exists := defaults["base-dev"]
	if !exists {
		t.Fatal("base-dev profile should exist")
	}

	var profile NexusProfile
	if err := yaml.Unmarshal([]byte(content), &profile); err != nil {
		t.Fatalf("failed to parse base-dev: %v", err)
	}

	if profile.Name != "base-dev" {
		t.Errorf("expected Name 'base-dev', got %q", profile.Name)
	}
	if profile.Version == "" {
		t.Error("base-dev should have a non-empty version")
	}
	if len(profile.Targets) == 0 {
		t.Error("base-dev should have at least one target")
	}
}

func TestBundledDefaults_DataScienceProfileFields(t *testing.T) {
	defaults := BundledDefaults()
	content, exists := defaults["data-science"]
	if !exists {
		t.Fatal("data-science profile should exist")
	}

	var profile NexusProfile
	if err := yaml.Unmarshal([]byte(content), &profile); err != nil {
		t.Fatalf("failed to parse data-science: %v", err)
	}

	if profile.Name != "data-science" {
		t.Errorf("expected Name 'data-science', got %q", profile.Name)
	}
	if profile.Version == "" {
		t.Error("data-science should have a non-empty version")
	}
	if profile.Extends != "base-dev" {
		t.Errorf("data-science should extend 'base-dev', got %q", profile.Extends)
	}
}

func TestBundledDefaults_EachTargetHasValidFamily(t *testing.T) {
	defaults := BundledDefaults()
	for name, content := range defaults {
		t.Run(name, func(t *testing.T) {
			var profile NexusProfile
			if err := yaml.Unmarshal([]byte(content), &profile); err != nil {
				t.Fatalf("failed to parse profile %q: %v", name, err)
			}
			for i, target := range profile.Targets {
				if !AllowedPackageFamilies[target.Family] {
					t.Errorf("profile %q targets[%d].family %q is not in AllowedPackageFamilies", name, i, target.Family)
				}
				if len(target.Packages) == 0 {
					t.Errorf("profile %q targets[%d] should have at least one package", name, i)
				}
			}
		})
	}
}

func TestBundledDefaults_Idempotent(t *testing.T) {
	first := BundledDefaults()
	second := BundledDefaults()

	// Same number of keys
	if len(first) != len(second) {
		t.Errorf("BundledDefaults should return consistent key count: first=%d, second=%d", len(first), len(second))
	}

	// Same keys and values
	for key, firstVal := range first {
		secondVal, exists := second[key]
		if !exists {
			t.Errorf("key %q present in first call but missing in second", key)
			continue
		}
		if firstVal != secondVal {
			t.Errorf("value for key %q differs between calls", key)
		}
	}

	// No extra keys in second
	for key := range second {
		if _, exists := first[key]; !exists {
			t.Errorf("key %q present in second call but missing in first", key)
		}
	}
}

// ---------------------------------------------------------------------------
// DefaultRemoteBaseURL
// ---------------------------------------------------------------------------

func TestDefaultRemoteBaseURL_ReturnsNonEmpty(t *testing.T) {
	url := DefaultRemoteBaseURL()
	if url == "" {
		t.Fatal("DefaultRemoteBaseURL() should return a non-empty string")
	}
}

func TestDefaultRemoteBaseURL_ContainsGitHub(t *testing.T) {
	url := DefaultRemoteBaseURL()
	if !strings.Contains(url, "github") && !strings.Contains(url, "raw") {
		t.Errorf("DefaultRemoteBaseURL() should contain 'github' or 'raw', got %q", url)
	}
}

func TestDefaultRemoteBaseURL_IsHTTPS(t *testing.T) {
	url := DefaultRemoteBaseURL()
	if !strings.HasPrefix(url, "https://") {
		t.Errorf("DefaultRemoteBaseURL() should use HTTPS, got %q", url)
	}
}

func TestDefaultRemoteBaseURL_MatchesDefaultRemoteURL(t *testing.T) {
	url := DefaultRemoteBaseURL()
	if url != DefaultRemoteURL {
		t.Errorf("DefaultRemoteBaseURL() should return DefaultRemoteURL; got %q, want %q", url, DefaultRemoteURL)
	}
}

func TestDefaultRemoteBaseURL_PassesURLValidation(t *testing.T) {
	url := DefaultRemoteBaseURL()
	if err := validateRemoteURL(url); err != nil {
		t.Errorf("DefaultRemoteBaseURL() should pass validateRemoteURL, got error: %v", err)
	}
}
