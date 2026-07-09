# ADR 007: Chezmoi Shell-Out Strategy

**Status:** Accepted
**Date:** 2026-07-07
**Deciders:** Nexus Core
**Phase:** V7 — Identity & Security (The Soul)

---

## Context

V7 introduces dotfile management via [Chezmoi](https://www.chezmoi.io/). The
Nexus engine must integrate Chezmoi's capabilities (init, apply, add, verify,
status, diff) into its existing architecture without compromising the Zero-Trust
security model that governs all subprocess execution.

Two integration strategies are possible:

1. **Bundle chezmoi** — embed the chezmoi binary into the Nexus release
   artifacts (or import it as a Go library via `go install`).
2. **Shell out to chezmoi** — invoke the chezmoi binary as a separate process
   through `SanitizeAndExecute`, the same gate used for `apt`, `pacman`,
   `dnf`, `wsl`, and every other command in the engine.

---

## Decision

**Shell out to chezmoi via `SanitizeAndExecute`. Do not bundle.**

The `chezmoi` binary is added to the existing `AllowedCommands` whitelist in
`internal/engine/execute.go` (alongside `git`, which `chezmoi init` shells out
to internally). All V7 dotfile operations — `Detect`, `Install`, `BindSource`,
`Apply`, `Status`, `Diff`, `Add`, `Verify`, `UnbindSource` — invoke `chezmoi`
through the same Zero-Trust gate that protects every other command in the
engine.

A small bootstrap shim (`nexus dotfiles install`) installs chezmoi via the
existing `Orchestrator` when it is missing, so users do not need a separate
install step.

---

## Rationale

### Why shell-out, not bundle

- **Update independence.** Chezmoi releases on its own cadence. Bundling
  forces us to track upstream versions, embed signed binaries per-platform,
  and re-validate them on every release. Shell-out means users (and our
  installer) get fresh chezmoi via their normal package manager.

- **Size.** A static chezmoi binary is ~20 MB. Bundling blows up the Nexus
  binary (currently ~10 MB) by 2-3x for a tool most users will invoke
  directly when they want it anyway.

- **Trust surface.** Bundling means shipping chezmoi's compiled code under
  our name. Any bug in chezmoi (e.g., a CVSS-rated issue in its template
  engine) becomes a Nexus CVE. Shell-out keeps the trust boundary clear:
  chezmoi bugs are chezmoi bugs.

- **License review burden.** Chezmoi is MIT-licensed and we are
  Apache-2.0. Bundling requires a NOTICE-file audit and binary attribution;
  shell-out keeps this trivial because we never ship chezmoi's bytes.

### Why not import as a Go library

Chezmoi does not currently expose a stable Go API for headless invocation
that covers all the operations we need (init with a bound source, managed
diff, verify). Its CLI is the supported contract. Even if the Go API existed,
importing it would couple our release cycle to theirs and pull in
chezmoi's transitive dependencies (template engines, archive readers, etc.)
into our binary.

### Why add `chezmoi` to the allowlist directly

`SanitizeAndExecute` already implements the four-layer Zero-Trust defense
(command whitelisting, argument sanitization, context timeout, direct
`execve`). Adding `chezmoi` to the allowlist (alongside `apt`, `pacman`,
`wsl`, `git`) is the standard mechanism for extending the engine's
subprocess surface. No new code path is introduced.

### Why not add a GitHub-releases fallback for install

Per the V7 plan, `nexus dotfiles install` uses the existing `Orchestrator`
to install chezmoi via the system package manager (`apt install chezmoi`,
`pacman -S chezmoi`, etc.). On systems where chezmoi is not in the OS repo
(e.g., minimal Alpine containers), the command surfaces the underlying
package-manager error directly.

A GitHub-releases fallback was considered but rejected for V7:

- It would require an additional SSRF-safe downloader path (the existing
  `internal/wsl/downloader.go` is WSL-specific).
- It would require pinning a chezmoi SHA256 in our code, which we'd then
  have to update on every chezmoi release.
- The package-manager install is the more secure default (it goes through
  the OS's package trust chain, not ours).

A fallback can be added in a future slice if user demand warrants it.

---

## Consequences

### Positive

- **Zero new attack surface.** V7 inherits all the Zero-Trust guarantees of
  the existing engine. No new subprocess execution paths, no new argument
  parsers, no new SSRF code (the URL validation lives in
  `internal/engine/netguard.go`, shared with the WSL downloader).
- **Consistent user experience.** `nexus dotfiles install` looks and behaves
  like `nexus install` — same Orchestrator flow, same audit log, same state
  tracking.
- **Minimal code.** The entire V7 dotfiles package (`internal/dotfiles/`) is
  ~700 lines including comments, tests, and platform-specific files.
- **V8/V9 readiness.** The data shape (`DotfilesState`) and the URL
  validation primitive (`engine.ValidateURLAgainstHosts`) were designed to
  support V8 (Git-Sync) and V9 (Secrets Vault / age) without further
  refactoring. Adding V8 means adding new `dotfiles` subcommands; it does
  not require rearchitecting the integration.

### Negative

- **Chezmoi must be installed for V7 commands to work.** Mitigated by
  `nexus dotfiles install` (auto-install via package manager) and clear
  error messages ("chezmoi not installed — run `nexus dotfiles install`").
- **Chezmoi version drift.** Different users will have different chezmoi
  versions. We do not currently pin a minimum version. Mitigated by
  `Detect()` parsing `chezmoi --version` and surfacing it in every
  command's output.
- **`chezmoi diff` / `chezmoi status` exit code 1 is meaningful.** POSIX
  convention: exit 1 means "differences found", not "error". Our V7 code
  translates this exit-1 into a placeholder message because the engine's
  `SanitizeAndExecute` only returns stdout on exit 0. Users who need the
  full diff output must run `chezmoi diff` directly. This is documented
  in `nexus dotfiles diff --help`.

### Neutral

- **No new Go dependencies.** The dotfiles package imports only
  `internal/engine`, `internal/installer`, `pkg/manifest`, and stdlib.
- **No changes to the audit log format.** V7 uses the existing
  `AuditEntry` fields (Action, Target, Result, PackageManager). V7 audit
  codes are: `DOTFILES_DETECT`, `DOTFILES_INSTALL`, `DOTFILES_INIT`,
  `DOTFILES_APPLY`, `DOTFILES_ADD`, `DOTFILES_REMOVE`, `DOTFILES_VERIFY`.
  (Currently emitted as Action strings; not yet promoted to constants.)

---

## Alternatives Considered

### A. Vendoring chezmoi as a Go library
**Rejected.** No stable Go API covers all required operations. Would couple
release cycles and pull in transitive dependencies.

### B. Implementing a minimal dotfile manager in-house
**Rejected.** Chezmoi has been audited by thousands of users over five years.
Re-implementing its template engine, encryption support, and cross-platform
path handling would be years of work to reach feature parity. V7's goal is
to make Chezmoi usable, not to replace it.

### C. Wrapping chezmoi in a separate daemon process
**Rejected.** Adds operational complexity (process supervision, IPC contract,
lifecycle management) for no security benefit over shell-out.

### D. Using Ansible/Chef/Puppet for dotfile management
**Rejected.** These are configuration management tools aimed at server
fleets, not personal developer workstations. They require running a daemon
or agent, which contradicts the "zero runtime" philosophy.

---

## References

- [Chezmoi documentation](https://www.chezmoi.io/docs/)
- ADR 004: Zero-Trust Command Execution (the foundation this ADR builds on)
- ADR 001: Humble Object Pattern (the CLI/runner split that V7 follows)
- `internal/engine/execute.go` — `SanitizeAndExecute`, `AllowedCommands`
- `internal/engine/netguard.go` — `ValidateURL`, `ValidateURLAgainstHosts`
- `internal/dotfiles/` — V7 implementation
