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
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// AllowedSourceHosts is the closed-set whitelist of hosts from which dotfile
// sources may be cloned. Per the V7 plan ("D4: Source URL validation"):
// adding a new host requires explicit intent — this is the supply-chain
// boundary for the user's identity layer.
//
// These hosts are common Git hosting providers. They are trusted only
// insofar as the user can audit the dotfile repository at that URL; the
// whitelist itself does not vouch for repository contents.
var AllowedSourceHosts = map[string]bool{
	"github.com":    true,
	"gitlab.com":    true,
	"bitbucket.org": true,
	"codeberg.org":  true,
}

// SourceDeps holds dependencies required to bind/unbind a dotfile source.
// Mirrors the dependency-injection pattern used elsewhere in the engine.
type SourceDeps struct {
	ExecFn ExecFunc
	State  *engine.StateTracker
	Audit  *engine.AuditLogger
}

// NormalizeSourceURL converts the various accepted dotfile source URL forms
// into a single canonical form that ValidateURLAgainstHosts can process.
//
// Accepted inputs:
//   - "https://host/path"             → unchanged
//   - "ssh://git@host/path"           → unchanged
//   - "git@host:path"                 → "ssh://git@host/path"  (SCP-style)
//
// Returns the original string if it cannot be normalized (the caller will
// then get a validation error, which is the right outcome for garbage input).
func NormalizeSourceURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}

	// Already a URL with a scheme — leave it alone (validation will catch bad schemes).
	if strings.Contains(raw, "://") {
		return raw
	}

	// SCP-style form: user@host:path
	// We require an "@" before the first ":" to distinguish from a Windows path.
	at := strings.Index(raw, "@")
	colon := strings.Index(raw, ":")
	if at > 0 && colon > at {
		user := raw[:at]
		host := raw[at+1 : colon]
		path := raw[colon+1:]
		if user == "" || host == "" || path == "" {
			return raw // malformed SCP — let validation report it
		}
		// Strip leading slashes from the path (git's SCP form has no leading slash).
		path = strings.TrimPrefix(path, "/")
		return "ssh://" + user + "@" + host + "/" + path
	}

	return raw
}

// BindSource validates the source URL, performs a DNS-level SSRF check,
// runs `chezmoi init <repo>` to clone and bind the repository, and records
// the binding in state.
//
// Validation layers (all must pass):
//  1. NormalizeSourceURL — convert SCP-style (git@host:path) to ssh:// URL.
//  2. engine.ValidateURLAgainstHosts — HTTPS or SSH scheme, host in
//     AllowedSourceHosts, SSH userinfo must be "git".
//  3. checkHostNotPrivate — DNS-resolves the host and rejects any private IP,
//     defending against whitelisted hosts that resolve to internal addresses.
//
// On success: the user's chezmoi state (~/.local/share/chezmoi) now tracks
// the bound repository. Subsequent `nexus dotfiles apply` operations will
// pull from this source.
//
// On failure: chezmoi is not modified (chezmoi's own init is atomic — it
// either succeeds or leaves the previous state intact).
func BindSource(ctx context.Context, source string, deps SourceDeps) error {
	if deps.ExecFn == nil {
		return fmt.Errorf("dotfiles: SourceDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}

	source = NormalizeSourceURL(source)

	if err := engine.ValidateURLAgainstHosts(source, AllowedSourceHosts); err != nil {
		return fmt.Errorf("invalid source URL: %w", err)
	}

	if err := checkHostNotPrivate(source); err != nil {
		return fmt.Errorf("source URL rejected: %w", err)
	}

	if _, err := deps.ExecFn(ctx, "chezmoi", "init", source); err != nil {
		return fmt.Errorf("chezmoi init failed: %w", err)
	}

	if deps.State != nil {
		// State recording is best-effort: the bind already succeeded.
		// A failure here does not roll back the chezmoi init.
		_ = deps.State.RecordDotfilesInit(source)
	}

	return nil
}

// UnbindSource removes the dotfile source binding from chezmoi.
//
// SAFETY: this function does NOT delete chezmoi's on-disk state
// (~/.local/share/chezmoi). It only invokes `chezmoi init` (with no args)
// which chezmoi treats as a no-op when a source is already bound.
//
// To fully clean up after unbinding, the user must manually remove the
// chezmoi source directory. We deliberately do not automate this — a
// silent `rm -rf` of user data is exactly the kind of destructive
// behavior Zero-Trust is designed to prevent.
func UnbindSource(ctx context.Context, deps SourceDeps) error {
	if deps.ExecFn == nil {
		return fmt.Errorf("dotfiles: SourceDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}

	// chezmoi has no native "unbind" command. The closest safe equivalent
	// is to run `chezmoi init` with no args, which chezmoi treats as a
	// re-init that preserves the existing source directory.
	// We accept this as a no-op marker and rely on state-clearing below
	// to remove the binding from Nexus's perspective.
	_, _ = deps.ExecFn(ctx, "chezmoi", "init")

	if deps.State != nil {
		_ = deps.State.RecordDotfilesRemove()
	}

	return nil
}

// checkHostNotPrivate DNS-resolves the URL's hostname and verifies that
// none of the returned IPs fall in a private/reserved range. This is the
// second SSRF layer on top of URL validation: even a whitelisted host
// like github.com should not be allowed to redirect us to 127.0.0.1
// (DNS rebinding) or 169.254.169.254 (cloud metadata) via a malicious
// resolver.
//
// We use net.DefaultResolver.LookupIPAddr to honor the same DNS settings
// (and any future context timeouts) as the rest of the engine.
func checkHostNotPrivate(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("no hostname in URL")
	}

	ips, err := net.DefaultResolver.LookupIPAddr(context.Background(), host)
	if err != nil {
		return fmt.Errorf("DNS lookup failed for '%s': %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("hostname '%s' did not resolve to any IP", host)
	}

	for _, ip := range ips {
		if engine.IsPrivateIP(ip.IP) {
			return fmt.Errorf("hostname '%s' resolves to private/reserved IP %s (rejected)", host, ip.IP)
		}
	}
	return nil
}
