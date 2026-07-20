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
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ValidateURL — covers every reject path and the happy path
// ---------------------------------------------------------------------------

func TestValidateURL_HTTPS_HappyPath(t *testing.T) {
	cases := []string{
		"https://github.com/user/repo.git",
		"https://example.com",
		"https://example.com/path/to/file.yaml",
		"https://raw.githubusercontent.com/owner/repo/HEAD/file",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			if err := ValidateURL(u); err != nil {
				t.Errorf("ValidateURL(%q) = %v, want nil", u, err)
			}
		})
	}
}

func TestValidateURL_SSH_HappyPath(t *testing.T) {
	// SSH URLs must use the "git" user (git server convention).
	cases := []string{
		"ssh://git@github.com/user/repo.git",
		"ssh://git@gitlab.com/user/repo.git",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			if err := ValidateURL(u); err != nil {
				t.Errorf("ValidateURL(%q) = %v, want nil", u, err)
			}
		})
	}
}

func TestValidateURL_RejectsNonHTTPSOrSSH(t *testing.T) {
	cases := []string{
		"http://example.com",     // plain http
		"ftp://example.com/file", // ftp
		"file:///etc/passwd",     // local file
		"javascript:alert(1)",    // javascript scheme
		"data:text/plain,hello",  // data URL
		"ws://example.com",       // websocket
		"wss://example.com",      // websocket secure (not in allowlist)
		"",                       // empty
		"not-a-url",              // garbage
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			err := ValidateURL(u)
			if err == nil {
				t.Errorf("ValidateURL(%q) should have failed", u)
			}
		})
	}
}

func TestValidateURL_RejectsHTTPSUserinfo(t *testing.T) {
	err := ValidateURL("https://user:pass@example.com")
	if err == nil {
		t.Fatal("ValidateURL should reject HTTPS URLs with userinfo")
	}
	if !strings.Contains(err.Error(), "userinfo") {
		t.Errorf("error should mention 'userinfo', got: %v", err)
	}
}

func TestValidateURL_RejectsSSHMissingUserinfo(t *testing.T) {
	err := ValidateURL("ssh://github.com/user/repo.git")
	if err == nil {
		t.Fatal("ValidateURL should reject SSH URLs without userinfo")
	}
	if !strings.Contains(err.Error(), "userinfo") {
		t.Errorf("error should mention 'userinfo', got: %v", err)
	}
}

func TestValidateURL_RejectsSSHNonGitUser(t *testing.T) {
	err := ValidateURL("ssh://admin@github.com/user/repo.git")
	if err == nil {
		t.Fatal("ValidateURL should reject SSH URLs with non-git user")
	}
	if !strings.Contains(err.Error(), "git") {
		t.Errorf("error should mention 'git' user requirement, got: %v", err)
	}
}

func TestValidateURL_RejectsSSHPassword(t *testing.T) {
	err := ValidateURL("ssh://git:secret@github.com/user/repo.git")
	if err == nil {
		t.Fatal("ValidateURL should reject SSH URLs with password")
	}
	if !strings.Contains(err.Error(), "password") {
		t.Errorf("error should mention 'password', got: %v", err)
	}
}

func TestValidateURL_RejectsQueryParameters(t *testing.T) {
	err := ValidateURL("https://example.com/path?token=secret")
	if err == nil {
		t.Fatal("ValidateURL should reject URLs with query parameters")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("error should mention 'query', got: %v", err)
	}
}

func TestValidateURL_RejectsFragments(t *testing.T) {
	err := ValidateURL("https://example.com/path#fragment")
	if err == nil {
		t.Fatal("ValidateURL should reject URLs with fragments")
	}
	if !strings.Contains(err.Error(), "fragment") {
		t.Errorf("error should mention 'fragment', got: %v", err)
	}
}

func TestValidateURL_RejectsMissingHostname(t *testing.T) {
	err := ValidateURL("https:///path/file")
	if err == nil {
		t.Fatal("ValidateURL should reject URLs without hostname")
	}
	if !strings.Contains(err.Error(), "hostname") {
		t.Errorf("error should mention 'hostname', got: %v", err)
	}
}

func TestValidateURL_RejectsMalformedURL(t *testing.T) {
	// url.Parse is very lenient, but a control character should fail.
	err := ValidateURL("https://example.com\x00")
	if err == nil {
		t.Fatal("ValidateURL should reject URLs with control characters")
	}
}

// ---------------------------------------------------------------------------
// ValidateURLAgainstHosts — host whitelist enforcement
// ---------------------------------------------------------------------------

func TestValidateURLAgainstHosts_HappyPath(t *testing.T) {
	allowed := map[string]bool{
		"github.com":    true,
		"gitlab.com":    true,
		"bitbucket.org": true,
	}
	err := ValidateURLAgainstHosts("https://github.com/user/repo.git", allowed)
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestValidateURLAgainstHosts_NilAllowList(t *testing.T) {
	// Nil/empty allowed hosts must fail-closed (reject everything).
	err := ValidateURLAgainstHosts("https://github.com/user/repo.git", nil)
	if err == nil {
		t.Fatal("ValidateURLAgainstHosts with nil list should reject")
	}
	if !strings.Contains(err.Error(), "no allowed hosts") {
		t.Errorf("error should mention 'no allowed hosts', got: %v", err)
	}
}

func TestValidateURLAgainstHosts_EmptyAllowList(t *testing.T) {
	err := ValidateURLAgainstHosts("https://github.com/user/repo.git", map[string]bool{})
	if err == nil {
		t.Fatal("ValidateURLAgainstHosts with empty list should reject")
	}
}

func TestValidateURLAgainstHosts_RejectsDisallowedHost(t *testing.T) {
	allowed := map[string]bool{
		"github.com": true,
	}
	err := ValidateURLAgainstHosts("https://evil.com/repo.git", allowed)
	if err == nil {
		t.Fatal("ValidateURLAgainstHosts should reject non-allowlisted host")
	}
	if !strings.Contains(err.Error(), "not in the allowed list") {
		t.Errorf("error should mention 'not in the allowed list', got: %v", err)
	}
}

func TestValidateURLAgainstHosts_PropagatesValidateURLErrors(t *testing.T) {
	// An invalid URL (wrong scheme) should fail ValidateURL first.
	allowed := map[string]bool{"github.com": true}
	err := ValidateURLAgainstHosts("http://github.com/repo.git", allowed)
	if err == nil {
		t.Fatal("ValidateURLAgainstHosts should reject non-HTTPS URLs")
	}
}

// ---------------------------------------------------------------------------
// IsPrivateIP — covers every CIDR in privateRanges
// ---------------------------------------------------------------------------

func TestIsPrivateIP_Private(t *testing.T) {
	cases := []string{
		"10.0.0.1",        // RFC 1918 (10/8)
		"172.16.5.4",      // RFC 1918 (172.16/12)
		"192.168.1.1",     // RFC 1918 (192.168/16)
		"127.0.0.1",       // loopback
		"169.254.169.254", // AWS metadata / link-local
		"::1",             // IPv6 loopback
		"fc00::1",         // IPv6 unique local
		"fe80::1",         // IPv6 link-local
	}
	for _, ip := range cases {
		t.Run(ip, func(t *testing.T) {
			parsed := net.ParseIP(ip)
			if parsed == nil {
				t.Fatalf("failed to parse %q", ip)
			}
			if !IsPrivateIP(parsed) {
				t.Errorf("IsPrivateIP(%q) = false, want true", ip)
			}
		})
	}
}

func TestIsPrivateIP_Public(t *testing.T) {
	cases := []string{
		"8.8.8.8",              // Google DNS
		"1.1.1.1",              // Cloudflare DNS
		"93.184.216.34",        // example.com (historical)
		"2606:4700:4700::1111", // Cloudflare DNS IPv6
		"2001:4860:4860::8888", // Google DNS IPv6
	}
	for _, ip := range cases {
		t.Run(ip, func(t *testing.T) {
			parsed := net.ParseIP(ip)
			if parsed == nil {
				t.Fatalf("failed to parse %q", ip)
			}
			if IsPrivateIP(parsed) {
				t.Errorf("IsPrivateIP(%q) = true, want false", ip)
			}
		})
	}
}

func TestIsPrivateIP_NilIP(t *testing.T) {
	if IsPrivateIP(nil) {
		t.Error("IsPrivateIP(nil) should be false")
	}
}

func TestIsPrivateIP_IPv4InIPv6(t *testing.T) {
	// net.ParseIP returns a 16-byte form for IPv4 — make sure the
	// matching still works on the IPv4-in-IPv6 representation.
	ip := net.ParseIP("192.168.1.1")
	if !IsPrivateIP(ip) {
		t.Error("IsPrivateIP should match IPv4 addresses in their IPv4-in-IPv6 form")
	}
}

// ---------------------------------------------------------------------------
// NewSSRFSafeTransport — verify the transport is constructed with sane defaults
// ---------------------------------------------------------------------------

func TestNewSSRFSafeTransport_BasicConfig(t *testing.T) {
	tr := NewSSRFSafeTransport()
	if tr == nil {
		t.Fatal("NewSSRFSafeTransport returned nil")
	}
	if tr.DialContext == nil {
		t.Error("DialContext should be set (SSRF defense)")
	}
	if tr.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 10s", tr.TLSHandshakeTimeout)
	}
	if tr.ResponseHeaderTimeout != 30*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 30s", tr.ResponseHeaderTimeout)
	}
	if tr.MaxIdleConns != 10 {
		t.Errorf("MaxIdleConns = %d, want 10", tr.MaxIdleConns)
	}
}

func TestNewSSRFSafeTransport_DialContextRejectsPrivate(t *testing.T) {
	tr := NewSSRFSafeTransport()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Attempt to dial a private IP directly. The DialContext must reject
	// without ever establishing a connection.
	conn, err := tr.DialContext(ctx, "tcp", "127.0.0.1:80")
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		t.Fatal("DialContext should reject connections to 127.0.0.1 (loopback)")
	}
	if !strings.Contains(err.Error(), "private") {
		t.Errorf("error should mention 'private', got: %v", err)
	}
}

func TestNewSSRFSafeTransport_UsedAsRoundTripper(t *testing.T) {
	// Sanity check: the returned transport satisfies http.RoundTripper.
	tr := NewSSRFSafeTransport()
	var _ http.RoundTripper = tr
}

// ---------------------------------------------------------------------------
// PrivateRanges — sanity check via IsPrivateIP (delegates to netutil)
// ---------------------------------------------------------------------------

func TestPrivateRanges_Populated(t *testing.T) {
	// Verify IsPrivateIP detects known private IPs, confirming
	// the CIDR ranges were loaded correctly in netutil.init().
	testCases := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"169.254.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, tc := range testCases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("failed to parse IP %s", tc.ip)
		}
		got := IsPrivateIP(ip)
		if got != tc.want {
			t.Errorf("IsPrivateIP(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// url.Parse sanity — guard against accidental dependency removals
// ---------------------------------------------------------------------------

func TestURLParse_BasicShape(t *testing.T) {
	// This is a smoke test — we use url.Parse indirectly through
	// ValidateURL. If url.Parse breaks, ValidateURL tests fail.
	u, err := url.Parse("https://github.com/foo/bar.git")
	if err != nil {
		t.Fatalf("url.Parse failed: %v", err)
	}
	if u.Scheme != "https" || u.Hostname() != "github.com" {
		t.Errorf("parsed URL unexpected: scheme=%q host=%q", u.Scheme, u.Hostname())
	}
}
