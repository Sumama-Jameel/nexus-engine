# ADR 012: Hardware Ledger (The Intelligence)

**Status:** Accepted
**Date:** 2026-07-09
**Deciders:** Nexus Core
**Phase:** V13 — The Hardware Ledger (The Intelligence)

## Context

Phases 1-4 are complete (V1-V12). The engine can probe hardware, install
packages, manage dotfiles, switch modes, and run containers. But every
`nexus init` is a **blind operation** — the engine has no memory of what
worked (or didn't) on similar hardware.

Per the V13 roadmap: *"A Feedback Loop where the app asks: 'Did your Wi-Fi
work?' and saves the answer to a JSON file. A simple local JSON database
that periodically syncs to your GitHub repo. You are building the Neural
Net of hardware compatibility without expensive AI."*

## Decision

Implement a **Hardware Ledger** — a bounded-ring record of hardware
configurations and their install outcomes, stored in `NexusState` and
exposed through a new `internal/ledger/` bounded context.

### Key design decisions

1. **No ML / no AI.** The ledger uses **statistical matching** (simple
   majority of matching records) rather than probabilistic models. V13
   is the data collection layer; ML is deferred to V15.

2. **Privacy-first.** `DeviceFingerprint` is SHA256 of non-PII hardware
   attributes (OS + Arch + CPUModel + CPUCores + GPU + RAMTotalMB).
   No hostname, no IP, no user data, no serial numbers. Community sync
   is **opt-in only** (`CommunitySyncEnabled` defaults to `false`).

3. **Follows existing patterns.** The ledger is a new `internal/ledger/`
   bounded context (like `internal/mode/`, `internal/container/`) with
   its own directory, types, and test file. State persistence uses the
   same `StateTracker` path (atomic tmp+rename, mutex-guarded).

4. **Bounded ring.** `HardwareLedger.Records` holds at most 100 entries.
   Oldest entries drop off when the ring is full. No unbounded growth.

5. **No new binary whitelist entries.** Ledger operations use existing
   allowed commands (`sha256sum` is not needed — Go's stdlib has SHA256)
   and the existing `netguard.go` SSRF-safe transport for community sync.

## State Schema

```go
type HardwareLedger struct {
    Records              []HardwareReport `json:"records"`
    LastAnalyzedAt       time.Time        `json:"last_analyzed_at,omitempty"`
    CommunitySyncEnabled bool             `json:"community_sync_enabled"`
    LastSyncedAt         time.Time        `json:"last_synced_at,omitempty"`
}

type HardwareReport struct {
    DeviceFingerprint string `json:"device_fingerprint"`
    OS                string `json:"os"`
    Arch              string `json:"arch"`
    Kernel            string `json:"kernel"`
    CPUModel          string `json:"cpu_model"`
    CPUCores          int    `json:"cpu_cores"`
    RAMTotalMB        int    `json:"ram_total_mb"`
    DiskTotalGB       float64 `json:"disk_total_gb"`
    GPU               string `json:"gpu"`
    IsWSL2            bool   `json:"is_wsl2"`
    PackageManager    string `json:"package_manager"`
    Success           bool   `json:"success"`
    ErrorMessage      string `json:"error_message,omitempty"`
    ProfileName       string `json:"profile_name"`
    RecordedAt        time.Time `json:"recorded_at"`
}
```

## CLI Surface

| Command | Purpose |
|---------|---------|
| `nexus ledger record` | Record current hardware state as a report |
| `nexus ledger status` | Show ledger stats (entries, last record, last sync) |
| `nexus ledger query <field>` | Query: "has this GPU been tested on fedora?" |
| `nexus ledger sync` | Push local ledger to community GitHub registry |
| `nexus ledger pull` | Pull community compatibility data |
| `nexus ledger check` | Check: "is my hardware known to work?" |

## Integration Points

- **`nexus init`** Step 6: after the REPORT step, automatically record a
  `HardwareReport` with the probe results and install outcome.
- **`nexus install`**: records success/failure per operation.
- **`nexus wsl setup`**: records WSL2-specific hardware + success status.

## Security

- Hardware report contains **zero PII**: no hostname, no IP, no username.
- Community sync uses the existing SSRF-safe `netguard.go` transport.
- Sync URL is validated through `ValidateURLAgainstHosts` with the same
  host whitelist used for dotfile source binding.
- `CommunitySyncEnabled` defaults to `false` — no data leaves the machine
  without explicit user consent (set via `nexus ledger sync --enable`).

## Consequences

**Positive:**
- Engine gains memory of what works — users get proactive warnings
- "Neural Net" foundation without expensive infrastructure
- Privacy-preserving design builds user trust
- Reuses existing patterns (state, netguard, cobra CLI)

**Negative:**
- Local ledger is only as useful as the community — value grows with users
- Hardware fingerprint is deterministic (same hardware = same fingerprint)
- Bounded ring means old data is dropped (acceptable for hardware trends)

## References

- V13 NexusOS_pathway.txt: "The Hardware Ledger (The Intelligence)"
- V1-V12: All prior ADRs for pattern consistency
- `internal/engine/netguard.go`: SSRF-safe transport
- `internal/engine/state.go`: StateTracker atomic persistence pattern
- `internal/mode/mode.go`: Existing DDD bounded context pattern
