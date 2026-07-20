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
	"fmt"
	"net"
	"net/http"
	"net/url"

	"github.com/Sumama-Jameel/nexus-engine/internal/netutil"
)

// ValidateURL enforces a basic SSRF-safe URL contract:
//   - HTTPS or SSH scheme (SSH added in V8 for git SSH-key auth)
//   - For HTTPS: no userinfo, no query parameters, no fragments, non-empty hostname
//   - For SSH: userinfo is required and must be exactly "git" (git's well-known
//     convention); no query, no fragments, non-empty hostname
//
// It does NOT enforce a host whitelist. Callers that need whitelisting
// (e.g., V7 dotfile source binding) should call ValidateURLAgainstHosts.
//
// This function was extracted from internal/wsl/downloader.go to be shared
// by every network-fetching subsystem. Per the V7 plan: "Extract the SSRF
// guard from internal/wsl/downloader.go into a shared netguard.go."
//
// V8 update: SSH URLs are now accepted as a peer to HTTPS. This is how
// developers with SSH keys actually use git — `git@github.com:user/repo.git`.
// The userinfo "git" is allowed because git's SSH server only accepts the
// "git" user, not arbitrary usernames.
func ValidateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	switch parsed.Scheme {
	case "https":
		if parsed.User != nil {
			return fmt.Errorf("URLs with userinfo are not allowed")
		}
	case "ssh":
		if parsed.User == nil {
			return fmt.Errorf("SSH URLs must have a userinfo (e.g., ssh://git@host/path)")
		}
		if parsed.User.Username() != "git" {
			return fmt.Errorf("SSH URLs must use the 'git' user (got %q)", parsed.User.Username())
		}
		if pw, hasPw := parsed.User.Password(); hasPw && pw != "" {
			return fmt.Errorf("SSH URLs must not have a password")
		}
	default:
		return fmt.Errorf("only HTTPS or SSH URLs are allowed (got '%s')", parsed.Scheme)
	}

	if parsed.RawQuery != "" {
		return fmt.Errorf("URLs with query parameters are not allowed")
	}

	if parsed.Fragment != "" {
		return fmt.Errorf("URLs with fragments are not allowed")
	}

	if parsed.Hostname() == "" {
		return fmt.Errorf("URL must have a valid hostname")
	}

	return nil
}

// ValidateURLAgainstHosts enforces ValidateURL AND a host whitelist.
// The hostname of the URL must be present in allowedHosts.
//
// allowedHosts is treated as a closed set: a nil or empty map rejects
// every host (fail-closed). This is intentional — callers should
// explicitly enumerate trusted hosts rather than fall through.
func ValidateURLAgainstHosts(rawURL string, allowedHosts map[string]bool) error {
	if err := ValidateURL(rawURL); err != nil {
		return err
	}

	if len(allowedHosts) == 0 {
		return fmt.Errorf("no allowed hosts configured — refusing URL")
	}

	parsed, _ := url.Parse(rawURL) // already validated by ValidateURL
	host := parsed.Hostname()
	if !allowedHosts[host] {
		allowed := make([]string, 0, len(allowedHosts))
		for h := range allowedHosts {
			allowed = append(allowed, h)
		}
		return fmt.Errorf("host '%s' is not in the allowed list: %v", host, allowed)
	}

	return nil
}

// NewSSRFSafeTransport returns an http.Transport that rejects connections to
// private/reserved IP ranges. Delegates to netutil.NewSSRFSafeTransport.
func NewSSRFSafeTransport() *http.Transport {
	return netutil.NewSSRFSafeTransport()
}

// IsPrivateIP reports whether an IP is in a private or reserved range.
// Delegates to netutil.IsPrivateIP for shared implementation.
func IsPrivateIP(ip net.IP) bool {
	return netutil.IsPrivateIP(ip)
}
