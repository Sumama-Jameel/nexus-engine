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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RegistryProfile describes a profile in the community registry.
type RegistryProfile struct {
	// Name is the profile identifier used with `nexus registry fetch <name>`.
	Name string `json:"name"`
	// Version is the semantic version string from the profile's metadata.
	Version string `json:"version"`
	// Author is the person or organisation that created the profile.
	Author string `json:"author"`
	// Description is a one-line summary of what the profile provides.
	Description string `json:"description"`
	// TargetFamilies lists the package manager families supported.
	TargetFamilies []string `json:"target_families"`
	// SourceURL is the raw GitHub URL from which the profile YAML can be fetched.
	SourceURL string `json:"source_url"`
	// SHA256 is the hex-encoded SHA256 digest of the profile content.
	SHA256 string `json:"sha256"`
}

// communityRegistryURL is the base URL for the community registry index.
// The registry index is a JSON file that lists all available profiles with
// their metadata. Each profile's YAML is fetched individually from the
// nexus-profiles repository on GitHub.
const communityRegistryURL = "https://raw.githubusercontent.com/Sumama-Jameel/nexus-profiles/main/registry.json"

// profileBaseURL is used to construct individual profile download URLs.
const profileBaseURL = "https://raw.githubusercontent.com/Sumama-Jameel/nexus-profiles/main/profiles"

// registryHTTPClient is an HTTP client using the SSRF-safe transport.
// Reused across calls to avoid connection churn.
var registryHTTPClient = &http.Client{
	Transport: NewSSRFSafeTransport(),
	Timeout:   15 * time.Second,
}

// ListRegistry fetches the community registry index and returns all
// available profiles. Returns an empty slice (not nil) when the registry
// is unreachable or empty.
func ListRegistry(ctx context.Context) ([]RegistryProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, communityRegistryURL, nil)
	if err != nil {
		return []RegistryProfile{}, fmt.Errorf("failed to create registry request: %w", err)
	}

	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return []RegistryProfile{}, fmt.Errorf("failed to fetch registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []RegistryProfile{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return []RegistryProfile{}, fmt.Errorf(
			"registry returned HTTP %d (expected 200)", resp.StatusCode,
		)
	}

	limited := io.LimitReader(resp.Body, 5*1024*1024)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return []RegistryProfile{}, fmt.Errorf("failed to read registry: %w", err)
	}

	var profiles []RegistryProfile
	if err := json.Unmarshal(raw, &profiles); err != nil {
		return []RegistryProfile{}, fmt.Errorf("invalid registry format: %w", err)
	}
	if profiles == nil {
		return []RegistryProfile{}, nil
	}
	return profiles, nil
}

// SearchRegistry filters the community registry by keyword. Matching is
// case-insensitive across name, author, description, and target families.
// An empty query returns all profiles.
func SearchRegistry(ctx context.Context, query string) ([]RegistryProfile, error) {
	profiles, err := ListRegistry(ctx)
	if err != nil {
		return profiles, err
	}

	if query == "" {
		return profiles, nil
	}

	q := strings.ToLower(query)
	matched := make([]RegistryProfile, 0, len(profiles))

	for _, p := range profiles {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Author), q) ||
			strings.Contains(strings.ToLower(p.Description), q) {
			matched = append(matched, p)
			continue
		}
		for _, f := range p.TargetFamilies {
			if strings.Contains(strings.ToLower(f), q) {
				matched = append(matched, p)
				break
			}
		}
	}

	return matched, nil
}

// FetchRegistryProfile downloads a profile YAML from the community registry
// by name, validates its SHA256 against the index, and returns the raw YAML
// bytes. Returns an error if the profile is not found or the digest does not
// match.
func FetchRegistryProfile(ctx context.Context, name string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("profile name is required")
	}

	profiles, err := ListRegistry(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list registry: %w", err)
	}

	var match *RegistryProfile
	for i := range profiles {
		if profiles[i].Name == name {
			match = &profiles[i]
			break
		}
	}
	if match == nil {
		return nil, fmt.Errorf("profile %q not found in registry", name)
	}

	sourceURL := match.SourceURL
	if sourceURL == "" {
		sourceURL = fmt.Sprintf("%s/%s.yaml", profileBaseURL, name)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetch request: %w", err)
	}

	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch profile %q: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("profile %q returned HTTP %d", name, resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, 1*1024*1024)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read profile %q: %w", name, err)
	}

	if match.SHA256 != "" {
		got := fmt.Sprintf("%x", sha256.Sum256(data))
		if got != match.SHA256 {
			return nil, fmt.Errorf(
				"SHA256 mismatch for profile %q: expected %s, got %s", name, match.SHA256, got,
			)
		}
	}

	return data, nil
}

// SubmitProfile validates a local profile YAML and returns a submission URL
// pointing to the nexus-profiles repository where the user can create a PR.
func SubmitProfile(data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("profile data is empty")
	}

	// Minimum size check: a valid profile must have at least name and version.
	if len(data) < 32 {
		return "", fmt.Errorf("profile data is too small (%d bytes)", len(data))
	}

	// Compute the SHA256 for the submission receipt.
	digest := fmt.Sprintf("%x", sha256.Sum256(data))

	instructions := fmt.Sprintf(
		"https://github.com/Sumama-Jameel/nexus-profiles/new/main/profiles\n\n"+
			"Submit your profile via pull request at:\n"+
			"  https://github.com/Sumama-Jameel/nexus-profiles\n\n"+
			"Profile SHA256: %s\n"+
			"Place your .yaml file in the profiles/ directory and update registry.json.",
		digest,
	)

	return instructions, nil
}

// FormatRegistryProfiles returns a human-readable table of registry profiles.
func FormatRegistryProfiles(profiles []RegistryProfile) string {
	if len(profiles) == 0 {
		return "  No profiles found in the registry.\n"
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "  %-24s %-12s %-20s %s\n", "NAME", "VERSION", "AUTHOR", "DESCRIPTION")
	fmt.Fprintf(&b, "  %s\n", strings.Repeat("─", 100))

	for _, p := range profiles {
		desc := p.Description
		if len(desc) > 45 {
			desc = desc[:42] + "..."
		}
		fmt.Fprintf(&b, "  %-24s %-12s %-20s %s\n", p.Name, p.Version, p.Author, desc)
	}
	fmt.Fprintf(&b, "\n  %d profile(s) total\n", len(profiles))
	return b.String()
}
