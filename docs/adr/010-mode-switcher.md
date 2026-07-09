# ADR 010: Mode Switcher

**Status:** Accepted
**Date:** 2026-07-09
**Deciders:** Nexus Core
**Phase:** V11 — The Visual Layer (The Face)

## Context

V1–V10 deliver the engine's capabilities as separate subsystems: `profile` (declarative YAML manifests), `dotfiles` (chezmoi source), `vault` (age encryption), `sync` (git push/pull), and the Tauri dashboard. What is missing is a higher-level abstraction that bundles these into a single, switchable, auditable unit. The master plan calls this "The One-Click Profile Switcher" (V11), and the architecture document (`docs/plan.md` § 7 — "The Modes") describes Gamer / Dev / Work modes as pre-hashed scripts bundled in the binary.

Today, switching environments means running several commands in sequence and remembering what services to toggle. There is no record of the "currently active mode," no atomic switch, no rollback, and the dashboard's Profile tab is read-only.

## Decision

Introduce `mode` as a first-class concept — **not** a profile alias. A mode bundles:

1. **A profile reference** (the target environment: `base-dev`, `base-gamer`, `base-work`).
2. **An optional dotfile source** (re-binds chezmoi when the mode is applied).
3. **A service allowlist action** (`stop_services`, `start_services`).
4. **A bounded set of OS tweaks** (CPU governor on Linux, power plan on Windows).

Modes are switched atomically through a single pipeline that records every step, with `nexus mode rollback` as the recovery path.

### Mode YAML schema

```yaml
name: gamer                          # required, unique
description: "Kill heavy services"   # required
profile: base-gamer                  # required, must exist in profile store
dotfiles_source: ""                  # optional, Git URL or local path
stop_services: [podman, docker]      # optional, must be in allowlist
start_services: []                   # optional, must be in allowlist
os_tweaks:                           # optional, bounded enum per OS
  linux:
    cpu_governor: performance        # powersave | balanced | performance
  windows:
    power_plan: high_performance     # powercfg GUID or alias
```

### Built-in modes

Three modes ship embedded in the binary via `//go:embed`:

| Name | Profile | Default services | OS tweaks |
|---|---|---|---|
| `dev` | `base-dev` | start `sshd` (if installed) | balanced |
| `gamer` | `base-gamer` | stop `podman`, `docker` | performance |
| `work` | `base-work` | start `sshd` | balanced |

Built-ins are immutable; users override per-machine by placing a file with the same name in `~/.nexus/modes/`.

### Service allowlist — Option C (hybrid)

Hardcoded safe list of service names that the engine is willing to start/stop:

- **Linux:** `podman`, `docker`, `sshd`, `cron`, `nginx`, `apache2`, `postgresql`, `mysql`, `redis-server`, `redis`, `fail2ban`, `ufw`, `firewalld`, `bluetooth`, `cups`
- **Windows:** `spooler`, `w32time`, `sshd`, `docker`, `com.docker.service`

Unlisted service names are **rejected by default** with a clear error. The `--allow-unlisted-services` flag lifts the restriction; every such action is recorded in the audit log under `MODE_APPLY_UNLISTED_SERVICE` with a `WARNING` severity.

### CLI surface

```
nexus mode list                       # built-ins + user modes, with active marker
nexus mode current                    # active mode name + last switch timestamp
nexus mode show <name>                # print mode YAML + resolved profile + service plan
nexus mode apply <name>               # atomic switch (--dry-run, --yes, --allow-unlisted-services)
nexus mode rollback                   # re-apply the previously active mode
nexus mode define <name>              # interactive wizard (mirrors profileCreate)
```

### Apply pipeline

Each step runs through the existing Zero-Trust gate (`engine.SanitizeAndExecute`). State is updated atomically per step; failures short-circuit without auto-rollback (user runs `mode rollback`).

1. Validate mode YAML against the embedded JSON Schema.
2. Resolve target profile via `store.LoadProfileWithExtends` (existing).
3. `dry-run`: print the full plan and exit.
4. Snapshot current `ActiveMode` → `LastMode*`.
5. Apply profile via `installer.NewOrchestrator` (existing, `--dry-run` honored).
6. If `dotfiles_source` set, re-bind via `dotfiles.BindSource` (existing).
7. Stop / start services via `systemctl` (Linux) / `sc` (Windows) — allowlist enforced.
8. Apply OS tweaks via `cpupower` (Linux) / `powercfg` (Windows).
9. Update state: `ActiveMode`, `LastModeSwitchAt`, `LastModeSwitchFrom`, `LastModeSwitchTo`.
10. Audit `MODE_APPLY` with mode name, outcome, step list.

### State schema extension (backwards-compatible)

```go
type ModeState struct {
    Active         string    `json:"active,omitempty"`
    Previous       string    `json:"previous,omitempty"`
    LastSwitchAt   time.Time `json:"last_switch_at,omitempty"`
    LastSwitchFrom string    `json:"last_switch_from,omitempty"`
    LastSwitchTo   string    `json:"last_switch_to,omitempty"`
    History        []string  `json:"history,omitempty"` // bounded ring, last 20
}

type NexusState struct {
    // ... existing fields ...
    Mode ModeState `json:"mode"`
}
```

### Dashboard

The read-only Profile tab is **replaced** by a Modes tab:

- Active mode badge in the header (always visible).
- Dropdown selector listing built-ins + user-defined modes.
- Two-click confirm: select from dropdown → modal "Apply mode 'gamer'? This will stop podman and docker." → Apply.
- Refresh state after apply; show "last switched 3m ago (from dev)" footer.
- Empty-state CTA when no user-defined modes exist: "Define your first mode" links to docs.

### IPC additions (Tauri)

- `mode` added to the subcommand whitelist.
- One new Tauri command `mode_apply(name: String)` in `src-tauri/src/main.rs`.
- No new path inputs — existing path-validation logic is sufficient.

### Audit codes

- `MODE_APPLY` — successful switch
- `MODE_APPLY_FAILED` — switch aborted
- `MODE_APPLY_UNLISTED_SERVICE` — warning when `--allow-unlisted-services` was used
- `MODE_DEFINE` — new mode created
- `MODE_ROLLBACK` — reverted to previous

## Consequences

### Positive

- V11 delivers immediate utility: one click in the dashboard flips the whole environment.
- Built-ins mean the dashboard is never empty on day one.
- Atomic state writes + audit log make every mode switch traceable.
- Service allowlist preserves the Zero-Trust boundary; the `--allow-unlisted-services` escape hatch keeps power users unblocked.
- Re-uses existing infrastructure (profile store, orchestrator, chezmoi binding, audit log, state tracker).

### Negative

- No automatic rollback on partial failure — user must run `mode rollback` explicitly. This is deliberate: it avoids cascading failures where rollback itself fails.
- Windows `sc` and `powercfg` need to be added to `engine.AllowedCommands` (small surface expansion).
- Three new audit codes add some complexity to log consumers.

### Deferred (out of scope for V11)

- Mode scheduling / cron-like triggers (V13 candidate).
- Mode marketplace / community registry (V15 candidate).
- Auto-rollback on partial failure.
- Mode diffing between versions.
- Per-mode environment variables.

## References

- ADR 004: Zero-Trust Command Execution
- ADR 007: Chezmoi Shell-Out Strategy
- ADR 008: Git-Sync Strategy
- ADR 009: Secrets Vault Strategy
- `docs/plan.md` § 7 — "The Modes"
- `docs/NexusOS_pathway.txt` — V11 definition
- `internal/dotfiles/sync.go` — pattern for `SyncDeps` / pipeline-with-deps
- `internal/dotfiles/vault.go` — pattern for `vault.go` subcommand split
