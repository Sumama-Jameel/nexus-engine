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

package dotfiles

import (
	"strings"
	"testing"
)

func TestScan_NoMatches(t *testing.T) {
	clean := []byte(`
# This is a clean config file
export PATH=/usr/bin:/bin
alias ll='ls -la'
[user]
    name = John Doe
`)
	matches := Scan(clean)
	if len(matches) != 0 {
		t.Errorf("expected no matches in clean content, got %d: %+v", len(matches), matches)
	}
}

func TestScan_AWSAccessKey(t *testing.T) {
	content := []byte("aws_access_key_id = AKIAIOSFODNN7EXAMPLE\n")
	matches := Scan(content)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d: %+v", len(matches), matches)
	}
	if matches[0].Pattern != "AWS Access Key ID" {
		t.Errorf("wrong pattern: %s", matches[0].Pattern)
	}
	if strings.Contains(matches[0].Snippet, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("snippet must NOT contain the raw secret value")
	}
	if !strings.Contains(matches[0].Snippet, "***REDACTED***") {
		t.Errorf("snippet must contain redaction marker, got: %q", matches[0].Snippet)
	}
	if matches[0].Line != 1 {
		t.Errorf("expected line 1, got %d", matches[0].Line)
	}
}

func TestScan_GitHubPATClassic(t *testing.T) {
	content := []byte("GITHUB_TOKEN=ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n")
	matches := Scan(content)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Pattern != "GitHub Personal Access Token (classic)" {
		t.Errorf("wrong pattern: %s", matches[0].Pattern)
	}
	if strings.Contains(matches[0].Snippet, "ghp_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Error("snippet leaked the raw token")
	}
}

func TestScan_GitHubFineGrainedPAT(t *testing.T) {
	// Fine-grained PATs are 82 chars after the "github_pat_" prefix.
	suffix := strings.Repeat("a", 82)
	content := []byte("token = github_pat_" + suffix + "\n")
	matches := Scan(content)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Pattern != "GitHub Fine-Grained Personal Access Token" {
		t.Errorf("wrong pattern: %s", matches[0].Pattern)
	}
}

func TestScan_PrivateKeyPEM(t *testing.T) {
	content := []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA...
-----END RSA PRIVATE KEY-----
`)
	matches := Scan(content)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Pattern != "Private Key (PEM block)" {
		t.Errorf("wrong pattern: %s", matches[0].Pattern)
	}
}

func TestScan_OpenSSHPrivateKey(t *testing.T) {
	content := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n")
	matches := Scan(content)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for OPENSSH private key, got %d", len(matches))
	}
	if matches[0].Pattern != "Private Key (PEM block)" {
		t.Errorf("wrong pattern: %s", matches[0].Pattern)
	}
}

func TestScan_SlackToken(t *testing.T) {
	content := []byte("SLACK_TOKEN=xoxb-1234567890-abcdefghij\n")
	matches := Scan(content)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Pattern != "Slack Token" {
		t.Errorf("wrong pattern: %s", matches[0].Pattern)
	}
}

func TestScan_GenericAPIKey(t *testing.T) {
	cases := []string{
		`api_key = "abcdefghijklmnop1234"`,
		`APIKEY: 'xyz123abc456def789ghi'`,
		`auth_token = "thisIsAVeryLongSecretValue123"`,
	}
	for _, line := range cases {
		matches := Scan([]byte(line))
		if len(matches) != 1 {
			t.Errorf("expected 1 match for %q, got %d", line, len(matches))
			continue
		}
		if matches[0].Pattern != "Generic API key assignment" {
			t.Errorf("wrong pattern for %q: %s", line, matches[0].Pattern)
		}
	}
}

func TestScan_ShortGenericKeyNotMatched(t *testing.T) {
	// The generic pattern requires 16+ char values. Short strings
	// (common in test fixtures and innocuous configs) must NOT match.
	content := []byte(`api_key = "short"`)
	matches := Scan(content)
	if len(matches) != 0 {
		t.Errorf("short value should not match generic pattern, got %d: %+v",
			len(matches), matches)
	}
}

func TestScan_MultipleMatchesInSameFile(t *testing.T) {
	content := []byte(`# config
AKIAIOSFODNN7EXAMPLE
ghp_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
-----BEGIN PRIVATE KEY-----
`)
	matches := Scan(content)
	if len(matches) != 3 {
		t.Errorf("expected 3 matches (AWS, GitHub, PEM), got %d: %+v",
			len(matches), matches)
	}
}

func TestScan_LineNumberIsAccurate(t *testing.T) {
	content := []byte("# line 1\n# line 2\nAKIAIOSFODNN7EXAMPLE\n# line 4\n")
	matches := Scan(content)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Line != 3 {
		t.Errorf("expected match on line 3, got line %d", matches[0].Line)
	}
}

func TestScan_EmptyContent(t *testing.T) {
	if matches := Scan(nil); matches != nil {
		t.Errorf("expected nil matches for nil content, got %v", matches)
	}
	if matches := Scan([]byte{}); matches != nil {
		t.Errorf("expected nil matches for empty content, got %v", matches)
	}
}

func TestScan_NeverLeaksRawSecret(t *testing.T) {
	// Defense-in-depth: even with multiple patterns and complex content,
	// no Match.Snippet should ever contain the raw secret value.
	secretValue := "AKIAIOSFODNN7EXAMPLE"
	content := []byte("export AWS_KEY=" + secretValue + "\n")
	matches := Scan(content)
	for _, m := range matches {
		if strings.Contains(m.Snippet, secretValue) {
			t.Errorf("LEAK: snippet contains raw secret %q: %s", secretValue, m.Snippet)
		}
	}
}
