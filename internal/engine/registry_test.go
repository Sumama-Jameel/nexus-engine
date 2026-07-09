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
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestFormatRegistryProfiles_Empty(t *testing.T) {
	s := FormatRegistryProfiles(nil)
	if len(s) == 0 {
		t.Fatal("expected non-empty string for nil profiles")
	}
}

func TestFormatRegistryProfiles_Single(t *testing.T) {
	p := []RegistryProfile{
		{
			Name:        "test",
			Version:     "1.0.0",
			Author:      "tester",
			Description: "a test profile",
		},
	}
	s := FormatRegistryProfiles(p)
	if !contains(s, "test") || !contains(s, "1.0.0") || !contains(s, "tester") {
		t.Fatalf("output missing expected fields:\n%s", s)
	}
}

func TestFormatRegistryProfiles_Multiple(t *testing.T) {
	profiles := []RegistryProfile{
		{Name: "alpha", Version: "1.0", Author: "a", Description: "first"},
		{Name: "beta", Version: "2.0", Author: "b", Description: "second"},
		{Name: "gamma", Version: "3.0", Author: "c", Description: "very long description that should be truncated in the table output"},
	}
	s := FormatRegistryProfiles(profiles)
	if !contains(s, "alpha") || !contains(s, "beta") || !contains(s, "gamma") {
		t.Fatalf("output missing expected profiles:\n%s", s)
	}
	if !contains(s, "3 profile(s) total") {
		t.Fatalf("output missing total count:\n%s", s)
	}
}

func TestSubmitProfile_EmptyData(t *testing.T) {
	_, err := SubmitProfile(nil)
	if err == nil {
		t.Fatal("expected error for nil data")
	}
}

func TestSubmitProfile_TooSmall(t *testing.T) {
	_, err := SubmitProfile([]byte("tiny"))
	if err == nil {
		t.Fatal("expected error for tiny data")
	}
}

func TestSubmitProfile_Valid(t *testing.T) {
	data := []byte("name: my-profile\nversion: 1.0.0\nauthor: me\n")
	result, err := SubmitProfile(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(result, "github.com/Sumama-Jameel/nexus-engine") {
		t.Fatalf("result missing expected URL:\n%s", result)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	if !contains(result, digest) {
		t.Fatalf("result missing SHA256 digest:\n%s", result)
	}
}

func TestSearchRegistry_EmptyQuery(t *testing.T) {
	ctx := context.Background()
	results, err := SearchRegistry(ctx, "")
	if err != nil {
		t.Logf("network unavailable (expected in CI): %v", err)
		return
	}
	_ = results
}

func TestSearchRegistry_ShortQuery(t *testing.T) {
	ctx := context.Background()
	results, err := SearchRegistry(ctx, "xy")
	if err != nil {
		t.Logf("network unavailable (expected in CI): %v", err)
		return
	}
	_ = results
}

func BenchmarkListRegistry(b *testing.B) {
	ctx := context.Background()
	for i := 0; i < b.N; i++ {
		_, _ = ListRegistry(ctx)
	}
}

func BenchmarkFormatRegistryProfiles(b *testing.B) {
	profiles := make([]RegistryProfile, 100)
	for i := range profiles {
		profiles[i] = RegistryProfile{
			Name:        fmt.Sprintf("profile-%d", i),
			Version:     "1.0.0",
			Author:      "bench",
			Description: "benchmark profile " + fmt.Sprintf("%d", i),
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FormatRegistryProfiles(profiles)
	}
}

func BenchmarkSubmitProfile(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = SubmitProfile(data)
	}
}

func TestSearchRegistry_MatchesByName(t *testing.T) {
	t.Skip("requires network access to community registry")
}

func TestFetchRegistryProfile_NotFound(t *testing.T) {
	ctx := context.Background()
	_, err := FetchRegistryProfile(ctx, "profile-that-does-not-exist-xyz")
	if err == nil {
		t.Fatal("expected error for non-existent profile")
	}
	t.Logf("got expected error (also acceptable if network unavailable): %v", err)
}

func TestFetchRegistryProfile_EmptyName(t *testing.T) {
	_, err := FetchRegistryProfile(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
