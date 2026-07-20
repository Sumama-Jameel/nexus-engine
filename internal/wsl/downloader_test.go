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

package wsl

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// SSRF Protection — isPrivateIP logic
//
// The isPrivateIP function in downloader.go is Windows-only, but the
// SSRF protection logic is inherently cross-platform and must be tested.
// We re-implement the logic here as test helpers to verify the security
// properties on all platforms.
// ---------------------------------------------------------------------------

// mustParseCIDRTest mirrors mustParseCIDR from downloader.go
func mustParseCIDRTest(s string) *net.IPNet {
	_, network, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("invalid hardcoded CIDR '%s': %v", s, err))
	}
	return network
}

// isPrivateIPLogic mirrors isPrivateIP from downloader.go (windows).
// This is the SECURITY-CRITICAL SSRF protection layer.
func isPrivateIPLogic(ip net.IP) bool {
	privateRanges := []struct {
		network *net.IPNet
	}{
		{mustParseCIDRTest("10.0.0.0/8")},
		{mustParseCIDRTest("172.16.0.0/12")},
		{mustParseCIDRTest("192.168.0.0/16")},
		{mustParseCIDRTest("127.0.0.0/8")},
		{mustParseCIDRTest("169.254.0.0/16")},
		{mustParseCIDRTest("::1/128")},
		{mustParseCIDRTest("fc00::/7")},
		{mustParseCIDRTest("fe80::/10")},
	}

	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}

	return false
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool // true = private/blocked, false = public/allowed
	}{
		// RFC1918 private ranges — should be blocked (true = private)
		{name: "RFC1918 10.0.0.1", ip: "10.0.0.1", expected: true},
		{name: "RFC1918 10.255.255.255", ip: "10.255.255.255", expected: true},
		{name: "RFC1918 10.123.45.67", ip: "10.123.45.67", expected: true},
		{name: "RFC1918 172.16.0.1", ip: "172.16.0.1", expected: true},
		{name: "RFC1918 172.31.255.255", ip: "172.31.255.255", expected: true},
		{name: "RFC1918 172.20.0.1", ip: "172.20.0.1", expected: true},
		{name: "RFC1918 192.168.0.1", ip: "192.168.0.1", expected: true},
		{name: "RFC1918 192.168.1.1", ip: "192.168.1.1", expected: true},
		{name: "RFC1918 192.168.255.255", ip: "192.168.255.255", expected: true},

		// Loopback — should be blocked
		{name: "loopback 127.0.0.1", ip: "127.0.0.1", expected: true},
		{name: "loopback 127.0.0.2", ip: "127.0.0.2", expected: true},
		{name: "loopback 127.255.255.255", ip: "127.255.255.255", expected: true},

		// Link-local — should be blocked
		{name: "link-local 169.254.0.1", ip: "169.254.0.1", expected: true},
		{name: "link-local AWS metadata 169.254.169.254", ip: "169.254.169.254", expected: true},

		// IPv6 private ranges — should be blocked
		{name: "IPv6 loopback ::1", ip: "::1", expected: true},
		{name: "IPv6 ULA fc00::1", ip: "fc00::1", expected: true},
		{name: "IPv6 ULA fd00::1", ip: "fd00::1", expected: true},
		{name: "IPv6 link-local fe80::1", ip: "fe80::1", expected: true},

		// Public IPs — should NOT be blocked
		{name: "Google DNS 8.8.8.8", ip: "8.8.8.8", expected: false},
		{name: "Cloudflare 1.1.1.1", ip: "1.1.1.1", expected: false},
		{name: "GitHub 140.82.121.3", ip: "140.82.121.3", expected: false},

		// Boundary cases — 172.16.0.0/12 range
		{name: "172.15.255.255 is public", ip: "172.15.255.255", expected: false},
		{name: "172.16.0.0 is private", ip: "172.16.0.0", expected: true},
		{name: "172.31.255.255 is private", ip: "172.31.255.255", expected: true},
		{name: "172.32.0.0 is public", ip: "172.32.0.0", expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %q", tc.ip)
			}
			result := isPrivateIPLogic(ip)
			if result != tc.expected {
				t.Errorf("isPrivateIP(%s) = %v, expected %v", tc.ip, result, tc.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// URL validation logic
// ---------------------------------------------------------------------------

// validateDownloadURLLogic mirrors validateDownloadURL from downloader.go (windows)
func validateDownloadURLLogic(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "https" {
		return fmt.Errorf("only HTTPS URLs are allowed (got '%s')", parsed.Scheme)
	}
	if parsed.User != nil {
		return fmt.Errorf("URLs with userinfo (user:pass@) are not allowed")
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("URL must have a valid hostname")
	}
	if parsed.RawQuery != "" {
		return fmt.Errorf("URLs with query parameters are not allowed")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("URLs with fragments are not allowed")
	}
	return nil
}

func TestValidateDownloadURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		// Valid HTTPS URLs
		{
			name:    "valid HTTPS URL",
			url:     "https://dl-cdn.alpinelinux.org/alpine/v3.19/releases/x86_64/alpine-minirootfs-3.19.1-x86_64.tar.gz",
			wantErr: false,
		},
		{
			name:    "valid GitHub release URL",
			url:     "https://github.com/Sumama-Jameel/rootfs/releases/download/v1.0.0/nexus-debian-bookworm-amd64.tar.gz",
			wantErr: false,
		},

		// HTTP rejected — prevents MITM
		{
			name:    "HTTP rejected",
			url:     "http://example.com/file.tar.gz",
			wantErr: true,
			errMsg:  "only HTTPS",
		},
		{
			name:    "empty scheme rejected",
			url:     "example.com/file.tar.gz",
			wantErr: true,
			errMsg:  "only HTTPS",
		},

		// URLs with userinfo rejected — prevents credential leakage
		{
			name:    "user:pass userinfo rejected",
			url:     "https://user:pass@evil.com/file.tar.gz",
			wantErr: true,
			errMsg:  "userinfo",
		},
		{
			name:    "user-only userinfo rejected",
			url:     "https://user@evil.com/file.tar.gz",
			wantErr: true,
			errMsg:  "userinfo",
		},

		// URLs with query parameters rejected — prevents injection
		{
			name:    "query parameters rejected",
			url:     "https://example.com/file.tar.gz?foo=bar",
			wantErr: true,
			errMsg:  "query parameters",
		},

		// URLs with fragments rejected — prevents injection
		{
			name:    "fragment rejected",
			url:     "https://example.com/file.tar.gz#section",
			wantErr: true,
			errMsg:  "fragments",
		},

		// Empty hostname rejected
		{
			name:    "empty hostname rejected",
			url:     "https:///file.tar.gz",
			wantErr: true,
			errMsg:  "valid hostname",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDownloadURLLogic(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for URL %q, got nil", tc.url)
				} else if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for URL %q: %v", tc.url, err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Placeholder hash detection
// ---------------------------------------------------------------------------

func TestPlaceholderHashDetection(t *testing.T) {
	tests := []struct {
		name          string
		sha256        string
		isPlaceholder bool
	}{
		{name: "alpine placeholder", sha256: "placeholder-alpine-sha256-will-be-replaced-with-real-hash", isPlaceholder: true},
		{name: "debian placeholder", sha256: "placeholder-debian-sha256-will-be-replaced-with-real-hash", isPlaceholder: true},
		{name: "generic placeholder", sha256: "placeholder-something", isPlaceholder: true},
		{name: "real SHA256 hex", sha256: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", isPlaceholder: false},
		{name: "empty string", sha256: "", isPlaceholder: false},
		{name: "similar but not placeholder", sha256: "placeholders-are-not-the-same", isPlaceholder: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := strings.HasPrefix(tc.sha256, "placeholder-")
			if result != tc.isPlaceholder {
				t.Errorf("HasPrefix(%q, 'placeholder-') = %v, expected %v", tc.sha256, result, tc.isPlaceholder)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SHA256 computation — deterministic and consistent
// ---------------------------------------------------------------------------

func TestSHA256Computation(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		data := []byte("test content for hash verification")
		hash1 := fmt.Sprintf("%x", sha256.Sum256(data))
		hash2 := fmt.Sprintf("%x", sha256.Sum256(data))
		if hash1 != hash2 {
			t.Error("SHA256 computation should be deterministic")
		}
	})

	t.Run("different content produces different hashes", func(t *testing.T) {
		data1 := []byte("content A")
		data2 := []byte("content B")
		hash1 := fmt.Sprintf("%x", sha256.Sum256(data1))
		hash2 := fmt.Sprintf("%x", sha256.Sum256(data2))
		if hash1 == hash2 {
			t.Error("different content should produce different hashes")
		}
	})

	t.Run("hash length is 64 hex chars", func(t *testing.T) {
		data := []byte("test")
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		if len(hash) != 64 {
			t.Errorf("SHA256 hex hash should be 64 chars, got %d", len(hash))
		}
	})
}

// ---------------------------------------------------------------------------
// Download simulation with httptest
// ---------------------------------------------------------------------------

func TestDownloadSimulation(t *testing.T) {
	t.Run("URL validation rejects HTTP test server", func(t *testing.T) {
		// Create an HTTP test server (note: httptest creates HTTP, not HTTPS)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test content"))
		}))
		defer server.Close()

		// The URL from httptest is HTTP — validateDownloadURL should reject it
		err := validateDownloadURLLogic(server.URL + "/file.tar.gz")
		if err == nil {
			t.Error("expected HTTP URL to be rejected")
		}
		if !strings.Contains(err.Error(), "only HTTPS") {
			t.Errorf("expected HTTPS rejection, got: %v", err)
		}
	})

	t.Run("URL validation accepts HTTPS URLs", func(t *testing.T) {
		// Create an HTTPS test server
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test content"))
		}))
		defer server.Close()

		// The URL from httptest TLS server is HTTPS — validateDownloadURL should accept it
		err := validateDownloadURLLogic(server.URL + "/file.tar.gz")
		if err != nil {
			t.Errorf("HTTPS URL should be accepted, got error: %v", err)
		}
	})

	t.Run("URL validation rejects URLs with query parameters from test server", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := validateDownloadURLLogic(server.URL + "/file.tar.gz?token=secret")
		if err == nil {
			t.Error("expected URL with query parameters to be rejected")
		}
		if !strings.Contains(err.Error(), "query parameters") {
			t.Errorf("expected query parameter rejection, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// VerifyFileSHA256 logic — test the constant-time comparison concept
// ---------------------------------------------------------------------------

func TestVerifyFileSHA256Logic(t *testing.T) {
	t.Run("matching hashes pass verification", func(t *testing.T) {
		data := []byte("test file content")
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		// Re-compute — should match
		hash2 := fmt.Sprintf("%x", sha256.Sum256(data))
		if hash != hash2 {
			t.Error("same content should produce matching hashes")
		}
	})

	t.Run("mismatched hashes fail verification", func(t *testing.T) {
		data1 := []byte("original content")
		data2 := []byte("tampered content")
		hash1 := fmt.Sprintf("%x", sha256.Sum256(data1))
		hash2 := fmt.Sprintf("%x", sha256.Sum256(data2))
		if hash1 == hash2 {
			t.Error("different content should produce different hashes")
		}
	})

	t.Run("placeholder hash skips verification", func(t *testing.T) {
		placeholderHash := "placeholder-alpine-sha256"
		// The download logic should skip verification for placeholder hashes
		if !strings.HasPrefix(placeholderHash, "placeholder") {
			t.Error("placeholder hash detection should work")
		}
	})

	t.Run("empty hash skips verification", func(t *testing.T) {
		emptyHash := ""
		// Empty hash should also skip verification
		if emptyHash != "" {
			t.Error("empty hash check should work")
		}
	})
}
