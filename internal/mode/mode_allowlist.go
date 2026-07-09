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

// Package mode is the V11 bounded context for atomic mode switching.
// A mode bundles a profile reference, optional dotfiles overlay, service
// toggles, and bounded OS tweaks into a single switchable, auditable unit.
//
// Per ADR 010: modes ship as built-ins embedded in the binary (//go:embed)
// plus per-machine overrides at ~/.nexus/modes/*.yaml. The service
// allowlist is the Zero-Trust gate for which systemd / sc service names
// the engine is willing to start or stop — see mode_allowlist.go.
package mode

// DefaultServiceAllowlist is the hardcoded set of service names the engine
// is willing to start/stop without an explicit escape hatch.
//
// Per ADR 010 § "Service allowlist — Option C": the safe list is the
// default. Operators may pass --allow-unlisted-services to lift the
// restriction; every unlisted action is recorded in the audit log under
// MODE_APPLY_UNLISTED_SERVICE with a WARNING result so the action is
// traceable in incident review.
//
// Linux set: common server / workstation services that are reasonable to
// toggle on a developer machine. We deliberately exclude critical system
// services (network-manager, systemd-journald, sshd is included but
// flagged in docs) so a typo in a mode YAML cannot silently brick the box.
//
// Windows set: matches the SC-compatible service names; note that sc.exe
// uses the registry key name, not the display name.
var DefaultServiceAllowlist = map[string]map[string]bool{
	"linux": {
		"podman":      true,
		"docker":      true,
		"sshd":        true,
		"cron":        true,
		"nginx":       true,
		"apache2":     true,
		"httpd":       true,
		"postgresql":  true,
		"mysql":       true,
		"mariadb":     true,
		"redis":       true,
		"redis-server": true,
		"fail2ban":    true,
		"ufw":         true,
		"firewalld":   true,
		"bluetooth":   true,
		"cups":        true,
	},
	"windows": {
		"spooler":            true,
		"w32time":            true,
		"sshd":               true,
		"docker":             true,
		"com.docker.service": true,
	},
}

// IsServiceAllowed reports whether the given service name is in the
// allowlist for the given OS. goos must be one of "linux", "windows".
// Unknown OS strings return false — fail closed.
func IsServiceAllowed(name, goos string) bool {
	set, ok := DefaultServiceAllowlist[goos]
	if !ok {
		return false
	}
	return set[name]
}

// ErrServiceNotAllowed is returned by validateServices when a service name
// is not in the allowlist and --allow-unlisted-services was not set.
type ErrServiceNotAllowed struct {
	Service string
	GOOS    string
}

func (e *ErrServiceNotAllowed) Error() string {
	return "service '" + e.Service + "' is not in the allowlist for " + e.GOOS +
		" — refusing to " + actionVerb() + " an unlisted service. " +
		"Pass --allow-unlisted-services to override (audit-logged)."
}

// actionVerb is used in error messages — kept as a tiny helper so the
// allowlist package has no command-aware context.
func actionVerb() string { return "manage" }
