# ADR 011: Container Sandbox (Distrobox)

**Status:** Accepted
**Date:** 2026-07-09
**Deciders:** Nexus Core
**Phase:** V12 — The App Sandbox (The Container)

## Context

V11 delivers mode switching (profile + services + OS tweaks). The next step is
container management: users need to run apps from any Linux distribution
without leaving their host environment. Distrobox (via Podman) already exists
in `AllowedCommands` — V12 wires it into a proper product surface with state
tracking, audit logging, and auto-cleanup on state write failure.

## Decision

Use Distrobox as the primary container interface (not raw Podman). Distrobox
provides:
- Seamless host integration (HOME, display, audio)
- Automatic container lifecycle management
- `distrobox-list --json` for programmatic status
- Built-in app export

V12 ships `distrobox-create` / `distrobox-rm` / `distrobox-list` through the
existing `SanitizeAndExecute` gate. No new binaries in the whitelist.

## Design

### Auto-cleanup (critical)

If `distrobox-create` succeeds but the state write fails, the engine
**auto-removes** via `distrobox-rm --force <name>` and returns a wrapped
error: `"container %q created but state write failed (%w); auto-removed"`.
This keeps `ContainerState` consistent — `nexus container list` always shows
what Nexus actually manages. Audit `CONTAINER_CREATE_ROLLED_BACK`.

### State schema

```go
type ContainerState struct {
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	Family    string    `json:"family"`
	CreatedAt time.Time `json:"created_at"`
}

type NexusState struct {
	// ... existing ...
	Containers map[string]ContainerState `json:"containers,omitempty"`
}
```

Adds to `NexusState`, zero-value safe. No migration needed.

### CLI

```
nexus container create <name> --image <image>       # create
nexus container list                                 # list managed
nexus container info <name>                         # show details
nexus container enter <name>                        # print command
nexus container apps <name>                         # list apps
nexus container remove <name>                       # gated
```

### Linux-only

Distrobox is Linux-native. Windows users access via WSL2 (existing infrastructure).
No Windows-specific code in V12.

### Security

- Container names: `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$` — reject shell metachars
- Image refs: `^[a-zA-Z0-9][a-zA-Z0-9._/-]{1,127}(:[a-zA-Z0-9._-]{1,128})?$`
- `--force` for remove skips the managed-state check