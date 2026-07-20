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

// Package netutil provides shared network security primitives used by
// multiple packages (engine, manifest, wsl) to avoid circular imports.
// It contains the SSRF-safe HTTP transport and private IP detection.
package netutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// NewSSRFSafeTransport returns an http.Transport with a DialContext that
// rejects connections to private/reserved IP ranges BEFORE the connection
// is established. This is the SSRF protection layer for any HTTP fetcher
// in the engine.
//
// Rejected ranges (per the V5 downloader implementation):
//   - 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 (RFC 1918)
//   - 127.0.0.0/8 (Loopback)
//   - 169.254.0.0/16 (Link-local)
//   - ::1/128 (IPv6 loopback)
//   - fc00::/7 (IPv6 unique local)
//   - fe80::/10 (IPv6 link-local)
//
// Caller is responsible for setting an overall context timeout on the
// returned Client. The transport itself only sets per-phase timeouts.
func NewSSRFSafeTransport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address '%s': %w", addr, err)
			}

			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve '%s': %w", host, err)
			}

			for _, ip := range ips {
				if IsPrivateIP(ip.IP) {
					return nil, fmt.Errorf("resolved address %s is in a private/reserved range (rejected for security)", ip.IP)
				}
			}

			dialer := &net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
	}
}

// IsPrivateIP reports whether an IP is in a private or reserved range.
// Exported for callers that need to make their own SSRF decisions
// (e.g., the dotfile source resolver checks DNS before git-cloning).
func IsPrivateIP(ip net.IP) bool {
	for _, network := range privateRanges {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// privateRanges is the canonical set of private/reserved CIDR ranges
// that the Nexus engine refuses to connect to. It is initialized once
// at package load — if any CIDR is malformed, the package fails to
// compile (panic in init), which is the desired fail-fast behavior.
var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		"10.0.0.0/8",     // RFC 1918
		"172.16.0.0/12",  // RFC 1918
		"192.168.0.0/16", // RFC 1918
		"127.0.0.0/8",    // Loopback
		"169.254.0.0/16", // Link-local
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
	}
	for _, c := range cidrs {
		_, network, err := net.ParseCIDR(c)
		if err != nil {
			panic(fmt.Sprintf("netutil: invalid hardcoded CIDR '%s': %v", c, err))
		}
		privateRanges = append(privateRanges, network)
	}
}
