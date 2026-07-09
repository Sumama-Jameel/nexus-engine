# ADR 008: Git-Sync Strategy (V8 — The Git-Sync Engine)

**Status:** Accepted
**Date:** 2026-07-07
**Deciders:** Nexus Core
**Phase:** V8 — Identity & Security (The Soul)

---

## Context

V7 introduced dotfile binding (`nexus dotfiles init`) via Chezmoi. The
binding clones a Git repository and configures Chezmoi to track it, but
changes stay local to the machine that made them. V8 adds the missing
leg: **two-way sync with the remote**. Without V8, a user with two
machines (laptop + desktop) cannot keep their dotfiles in step — each
machine's V7 binding is an island.

Two integration strategies are possible:

1. **Use `go-git` (pure-Go Git library)** — embed Git operations directly
   in the Nexus binary. No external `git` dependency at runtime.
2. **Shell out to `git` / `chezmoi git`** — invoke the system `git` binary
   (already in the engine's `AllowedCommands` allowlist) through the same
   Zero-Trust gate as every other command.

Additionally, two authentication strategies are possible:

A. **SSH only** — require users to have an SSH key configured.
B. **SSH + HTTPS-PAT** — support both. PAT via env var or `--token` flag.

---

## Decision

### Strategy: Shell out to `git` via `chezmoi git`

**Use the system `git` binary through `chezmoi git`, exactly as V7 does.**
The `git` command is already in `AllowedCommands` (line 68 of
`internal/engine/execute.go`), so no allowlist changes are needed. All
V8 operations — `Push`, `Pull`, `Sync` — invoke `chezmoi git <subcommand>`
through `SanitizeAndExecute`, inheriting the same Zero-Trust guarantees
as V7.

### Strategy: SSH + HTTPS-PAT, PAT via env var or `--token` flag

**Support both authentication methods from day one.** Developers almost
universally have SSH keys configured; supporting SSH means "it just works"
for the common case. Supporting HTTPS-PAT covers users who bind via HTTPS
(in V7) or who prefer PAT-based workflows.

The PAT is **never persisted**:
- Read from `--token` flag (one-shot, in-memory only)
- Fall back to `NEXUS_DOTFILES_TOKEN` environment variable (set by user)
- Injected into the remote URL for the duration of a single exec call via
  `https://x-access-token:<token>@host/path` rewriting
- Never logged, never written to `state.json`, never written to
  `.git/config`

This is the smallest possible surface that covers the realistic cases.
OS keyring support (via `zalando/go-keyring` or similar) is explicitly
**out of scope for V8** — it adds a dependency and platform-specific
code without changing the security model meaningfully (the user still
trusts some piece of state to persist the token).

### Pre-push secret scan

Every `Push` (and `Sync`'s push step) runs a **regex-based secret
scanner** over staged file contents before committing. Detected patterns:
AWS access keys, AWS secret keys, GitHub PATs (classic + fine-grained),
PEM private keys, OpenSSH private keys, Slack tokens, generic API key
assignments.

The scanner **warns and refuses by default**. The `--force` flag
overrides the refusal, and the override is recorded in the audit log.
This is defense-in-depth — V7 already blocks sensitive paths by
default, but secrets can hide in non-sensitive files (a `.zshrc` with
an exported API key is the textbook mistake).

### Conflict policy: `--ff-only` by default

`Pull` (and `Sync`'s pull step) use `--ff-only` by default. This
**refuses to merge when the local working copy has diverged from the
remote**, forcing the user to inspect the conflict manually rather than
silently merging or rebasing. The `--rebase` flag opts into
auto-replay for the "single user, multiple machines" workflow where
divergence is usually benign.

This is the conservative default. Auto-merge for dotfiles is a recipe
for silent data loss (e.g., one machine overwrites another's SSH config
without the user noticing). The user can opt into rebase when they
understand the implications.

### Timeout: inherit `engine.CommandTimeoutSec` (60 seconds)

Push and pull use the same 60-second timeout as every other command
in the engine (set via `CommandTimeoutSec` in `internal/engine/execute.go`).
This is the realistic ceiling for normal dotfile repos on slow
connections. Users with multi-MB repos and very slow links may hit
this — that's a V8.1 problem, not a V8 blocker. Five-minute timeouts
were considered and rejected: a dotfile push that takes five minutes
indicates a misconfiguration, not a network condition, and we should
fail fast and surface the error.

---

## Rationale

### Why shell-out, not go-git

- **Zero new dependencies.** `go-git` is ~3 MB of pure-Go code with a
  deep transitive dep tree. The system `git` is already installed
  everywhere Nexus runs.
- **Inherits V7's Zero-Trust guarantees.** `SanitizeAndExecute` enforces
  command whitelisting, argument sanitization, and context timeout.
  go-git would require us to re-implement these gates in Go.
- **Same trust surface as V7.** Users already trust `git` to handle
  their dotfile repo's authentication, signing, and protocol details.
  Wrapping `chezmoi git` is a thin layer that adds nothing new.
- **Consistent user experience.** `nexus dotfiles push` and `git push`
  behave identically; users who debug with `git` directly get the
  same result.

### Why support both SSH and HTTPS-PAT

- **SSH is the common case.** Developers almost universally have SSH
  keys configured. Forcing HTTPS-PAT would be a regression from the
  V7 workflow where `git@github.com:user/repo.git` URLs "just work".
- **HTTPS-PAT covers the V7 HTTPS-bind path.** Users who bound via
  HTTPS in V7 need a way to push/pull without re-binding. PAT via
  `NEXUS_DOTFILES_TOKEN` or `--token` is the canonical mechanism.
- **Env var + flag is the minimum-viable token story.** Anything more
  (OS keyring, encrypted credential files) requires new dependencies
  and a key-management story. V9's Secrets Vault work is the right
  home for that.

### Why `--ff-only` is the default

- **Dotfiles are personal identity.** A silent overwrite of `~/.ssh/config`
  or `~/.gitconfig` can lock the user out of their own accounts.
- **Conflicts are rare in practice.** A user with two machines typically
  has one "primary" and one "secondary" — divergence happens once,
  the user inspects it once, and from then on they know to `pull`
  before `push`.
- **`--rebase` is one flag away.** Power users with predictable
  workflows can opt into rebase without changing the default.

### Why refuse secrets by default (rather than warn-and-push)

- **The audit log records everything, but the secret is already in
  the world by the time you audit.** Refusal is the only state that
  guarantees no secret leaves the machine.
- **`--force` is one flag away.** Power users with legitimate secret
  patterns (e.g., a test fixture containing a fake AWS key) can
  override — and the override is auditable.
- **False positives are tolerable.** Our scanner uses high-confidence
  patterns (fixed prefix + length) to keep false positives low. A
  user who hits a false positive learns to use `--force` for that
  specific case.

---

## Consequences

### Positive

- **Zero new dependencies.** No new `go.mod` entries. The V8 package
  is ~400 lines of Go stdlib + existing engine/installer imports.
- **Inherits all V7 security guarantees.** Command whitelisting,
  argument sanitization, context timeout, audit logging all apply to
  V8 commands unchanged.
- **State extensions are additive.** `LastPushedAt`, `LastPulledAt`,
  and `LastCommitSHA` are new fields on `DotfilesState`. V7 state
  files without these fields deserialize cleanly (zero values).
- **Schema migration is unnecessary.** No existing JSON shape changes;
  only new optional fields.

### Negative

- **PAT in process args.** Injecting the PAT into the URL means it
  appears in `ps` output during the brief exec call. We mitigate by:
  - Never persisting to `.git/config` (URL rewrite is one-shot)
  - Never logging the URL
  - Never including the URL in error messages
  - Documenting the trade-off in this ADR
- **Conflicting workflows require manual resolution.** `--ff-only`
  means users with active multi-machine workflows will see
  "non-fast-forward" errors and need to manually rebase or reset.
  This is by design but is the most likely source of V8 support tickets.
- **Secret scanner can produce false positives.** Patterns like
  `api_key = "..."` will match legitimate test fixtures. The
  `--force` flag is the workaround. A future improvement could
  add a per-file `// nexus-safe` directive.

### Neutral

- **Three new CLI commands, one new state field group.** Total
  surface area added: `nexus dotfiles push|pull|sync` and
  `DotfilesState.{LastPushedAt, LastPulledAt, LastCommitSHA}`.
- **One new package-level file (`internal/dotfiles/sync.go`).**
  The push/pull/sync primitives live there alongside the existing
  V7 primitives. The secret scanner is in its own file
  (`secretscan.go`) for testability and reuse.
- **No new Go dependencies.** The dotfiles package imports only
  `internal/engine`, `internal/installer`, `pkg/manifest`, and stdlib.

---

## Alternatives Considered

### A. Use `github.com/go-git/go-git/v5` (pure-Go Git library)

**Rejected.** Adds ~3 MB of code and a deep dep tree. Re-implements
security-critical operations (auth, signing, protocol) that the
system `git` already handles correctly. No clear win over shell-out.

### B. SSH-only authentication

**Rejected.** Forces V7 HTTPS-bound users to re-bind as SSH to push.
Significant UX regression for users who don't have SSH keys configured
(common on fresh Windows installs).

### C. Persist the PAT to the OS keyring via `zalando/go-keyring`

**Deferred to V9.** Adds a new dependency and platform-specific code.
The env var + flag mechanism covers the realistic V8 cases. V9's
Secrets Vault work is the right place to integrate with the keyring
(it's also where we'd handle SSH key signing).

### D. Auto-merge on conflict

**Rejected.** Silently resolves the kind of conflicts that destroy
dotfiles. A user who sees a merge error can investigate; a user who
doesn't notice a silent overwrite can lose SSH keys or git config.
Force-pull is one `--rebase` flag away.

### E. Auto-scan secrets on every file write (not just push)

**Rejected.** Performance overhead, false-positive risk, and unclear
benefit. The pre-push gate catches secrets at the only moment they
matter: the moment they would leave the machine.

---

## References

- ADR 007: Chezmoi Shell-Out Strategy (the foundation this ADR builds on)
- ADR 004: Zero-Trust Command Execution
- `internal/engine/execute.go` — `SanitizeAndExecute`, `AllowedCommands["git"]`
- `internal/engine/netguard.go` — `ValidateURL`, extended in V8 to allow `ssh://`
- `internal/dotfiles/sync.go` — Push, Pull, Sync
- `internal/dotfiles/secretscan.go` — Pre-push secret scanner
- [Git documentation: gitcredentials, https://x-access-token convention](https://git-scm.com/docs/gitcredentials)
- [Chezmoi documentation: chezmoi git](https://www.chezmoi.io/docs/user-guide/command-line-tools/chezmoi-git/)
