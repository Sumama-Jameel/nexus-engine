# Nexus Protocol

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/Sumama-Jameel/nexus-engine)](https://goreportcard.com/report/github.com/Sumama-Jameel/nexus-engine)
[![GoDoc](https://godoc.org/github.com/Sumama-Jameel/nexus-engine?status.svg)](https://godoc.org/github.com/Sumama-Jameel/nexus-engine)
[![Tests](https://img.shields.io/badge/Tests-917%20passing-brightgreen)](https://go.dev/)
[![Lines of Go](https://img.shields.io/badge/Go-41%2C456%20lines-blue)](https://github.com/Sumama-Jameel/nexus-engine)

**Open source alternative to Ansible, Distrobox, and shell scripts. One static binary. Zero dependencies. Any machine.**

---

## What This Solves

Setting up a development machine from scratch is a mess. Coder's 2025 State of Dev Environments survey found that **only 7% of organizations** can provision a machine in under an hour. The rest spend 2–3 hours hunting down the right incantations for their distro.

**You run different commands on Ubuntu vs Arch vs Fedora.** That's not a skill issue — that's a fragmentation problem. Linux's desktop market is split across **Debian/Ubuntu (49.9%), Arch (8.8%), and Fedora-family distros** — each with a different package manager, different defaults, different everything. Ask someone what `apt install` is on Arch and they'll say `pacman -S`. If they know.

**The tools that try to fix this make it worse:**

| Tool | Problem |
|---|---|
| **Shell scripts** | Fragile. One `apt update` failure in a 200-line script and the whole thing breaks. No rollback. No state. No security gate. |
| **Dotfile repos** | Great for config, useless for dependencies. Your `.zshrc` doesn't install `gcc`. |
| **Distrobox** | Needs a working Linux install first. Circular dependency — you need a working machine to set up a working machine. |
| **Ansible** | Built for server fleets, not one laptop. Playbooks are versioned, stateful, and expect SSH access to every target. |
| **Docker** | Needs a daemon. Needs sudo. Needs a running container. Zero help with your actual host environment. |

**Nexus is different.** One static Go binary (`CGO_ENABLED=0`, zero C deps, works on any amd64 or arm64 Linux) detects your OS, probes your hardware, picks the right package manager, and installs everything you need from a YAML profile — with a **7-step security gate** around every command. No scripts. No manual steps. No fragile shell logic.

It's 111 `.go` files, 41,456 lines of Go, with 36 test files (23,913 test lines). 14 git commits. 65 CLI handler functions. **11 Go packages, all passing.** Built to outlast every distro upgrade you'll ever have.

---

## What We Need

You.

Nexus is open source because the Linux desktop deserves a setup tool that's as reliable as the kernel, not as fragile as a shell script. Here's how you help:

- **Drop a profile.** Got a sick dev setup? Wrap it as a YAML profile and toss it in the registry. 10 built-in profiles ship with the binary — add yours.
- **Fix something.** Open an issue. Send a PR. The command gate, the orchestrator, the WSL2 bridge — every layer is auditable Go.
- **Tell a friend.** The person running `sudo apt install` followed by `sudo pacman -S` followed by `sudo dnf install` on three different machines.

That's it. That's the whole thing.

---

## Quick Start

```bash
# Install (choose one)
go install github.com/Sumama-Jameel/nexus-engine/cmd/nexus@latest
# -- or download a binary from https://github.com/Sumama-Jameel/nexus-engine/releases

# Verify integrity
sha256sum -c checksums.txt

# Initialize your environment
nexus init
```

On Windows, get Linux in one command:

```bash
nexus wsl setup    # download, import, and configure WSL2 in 60 seconds
nexus wsl enter    # jump into your new Linux environment
```

---

## Features

- **4 package managers, one interface.** `apt`, `pacman`, `dnf`, `apk` behind a single `PackageManager` interface. Write a profile once, apply it on any distro.
- **7-step orchestrator with automatic rollback.** PreFlight → RefreshIndex → Order → Execute → Verify → Record → Audit. If a foundation package fails, Nexus removes everything from that run — no partial installs.
- **65 CLI handlers** across 17 top-level commands. Every `nexus` subcommand (profile create, vault encrypt, container enter, dotfiles push, wsl setup) is a registered cobra command with its own run function.
- **Security-gated execution.** All shell commands pass through `SanitizeAndExecute` — an allowlist, metacharacter rejection, timeouts, and structured error handling. No shell command runs directly.
- **10 built-in profiles.** `go-dev`, `rust-dev`, `frontend-dev`, `base-dev`, `data-science`, `base-gamer`, `base-gamer-nvidia`, `base-gamer-amd`, `base-work`, `ethical-hacker`. All embedded via `go:embed`.
- **Distro-aware suggestions.** Profiles declare a target distro. Nexus checks your OS, warns on mismatches, and suggests WSL2 or Distrobox as a fallback.
- **Parallel package installation.** Packages install concurrently within priority groups (Foundation, Language, Tool). No serial `apt-get` chains.
- **Secure downloads.** HTTPS only. DNS-level SSRF blocking of private IPs. Host whitelisting. 1 MB response size limits. Schema validation before anything touches disk.
- **SHA256 integrity.** Constant-time comparison for every profile and rootfs image. No timing side-channels.
- **WSL2 setup in 60 seconds.** `nexus wsl setup` checks readiness, downloads a rootfs, imports it into WSL2, and writes a hardened `wsl.conf` with `noexec` automounts.
- **Dotfile management through chezmoi.** Bind a Git repo, push with pre-push secret scanning, pull with ff-only by default. `nexus dotfiles push` stages, scans, and commits in one shot.
- **age-encrypted vault.** Encrypt sensitive files with age. Keys never leave your machine. `nexus vault encrypt` and `nexus vault decrypt` through the same command interface.
- **5 operating modes.** Switch between base, development, hardened, container, and gaming modes. Each mode adjusts settings and available commands.
- **Distrobox containers.** Create, enter, delete, and list isolated Linux environments — all from the same binary.
- **Community registry.** List, search, fetch, and submit profiles through a global registry. `nexus profile fetch` and `nexus profile submit` from the CLI.
- **Hardware compatibility ledger.** Record, query, and sync hardware reports with privacy-first fingerprinting. Check if your hardware is known to work with Nexus.
- **Windows-to-WSL2 folder migration.** Symlink your Windows Documents, Desktop, Downloads, and Pictures into WSL2 with `nexus teleport`.
- **Crash-safe state.** Atomic writes (write to temp file, then rename). Append-only activity log. State files survive `kill -9`.
- **Statically linked binary.** `CGO_ENABLED=0`. Zero C dependencies. Works on any Linux (amd64 or arm64) and Windows.
- **Desktop GUI (optional).** A Tauri dashboard with 7 tabs (Sync, Vault, System, Profile, Modes, Containers, Registry) that talks to the Go engine.

---

## Architecture

```
                     ┌─────────────────────────────────────────────┐
                     │              NEXUS PROTOCOL                  │
                     │                                               │
                     │  +-----------+    +-----------+              │
                     │  |   BRAIN   |───|   BRIDGE   |              │
                     │  |  (Go)     |   |  (WSL2)   |              │
                     │  |           |   |           |              │
                     │  | Cobra    |   | Windows   |              │
                     │  | Viper    |   | → Linux   |              │
                     │  | Engine   |   | IPC       |              │
                     │  | 10       |   |           |              │
                     │  | profiles  |   |           |              │
                     │  +-----+-----+   +-----+-----+              │
                     │        |                 |                  │
                     │  +-----v-----+   +-----v-----------+       │
                     │  |   DNA     |   |   CONTAINER     |       │
                     │  | (YAML)    |   |  (Distrobox)    |       │
                     │  |           |   |                 |       │
                     │  | SHA256    |   | Reproducible    |       │
                     │  | Schema    |   | environments    |       │
                     │  | Validation|   |                 |       │
                     │  +-----------+   +-----------------+       │
                     │                                               │
                     │  Security: SanitizeAndExecute (command gate)  │
                     │  Integrity: SHA256 + JSON Schema + netguard    │
                     │  State: Atomic JSON + append-only log          │
                     └─────────────────────────────────────────────┘
```

**Bounded contexts:**

- **BRAIN** — Cobra CLI + Viper + Go engine. All commands route through here. 65 handler functions, 17 top-level commands. The `SanitizeAndExecute` command gate lives here.
- **DNA** — Declarative YAML profiles. Two-layer validation: JSON Schema first, Go struct validation second. 10 embedded profiles, SHA256-indexed.
- **BRIDGE** — WSL2 detection and Windows-Linux bridge. Cross-compiled: `_linux.go` stubs and `_windows.go` real implementations.
- **CONTAINER** — Distrobox management. Decoupled from the engine via the `StateTracker` interface — containers don't know about profiles, profiles don't know about containers.
- **VAULT** — age-encrypted file store. Keys are machine-bound. No key exchange, no remote storage.
- **REGISTRY** — Community profile marketplace. Search, fetch, submit. Pull from the registry, apply with `nexus profile apply`.

**The 5-step flow:** Probe → Validate → Apply → Configure → Report.

**The 7-step orchestrator:** PreFlight → RefreshIndex → Order → Execute → Verify → Record → Audit.

---

## Competitor Comparison

Nexus is the **open source alternative** to every tool on this list.

| Tool | Static Binary | Cross-distro | Security Gate | Auto-Rollback | Dotfiles | WSL2 | 1-command init |
|---|---|---|---|---|---|---|---|
| **Nexus** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| **Ansible** | No | Yes | No | No | No | No | No |
| **Distrobox** | No | Yes | No | No | No | No | No |
| **Docker** | No | Yes | No | No | No | No | No |
| **chezmoi** | Yes | Yes | No | No | Yes | No | No |
| **dotbot** | No | No | No | No | Yes | No | No |
| **Shell script** | No | No | No | No | No | No | No |
| **Nix** | No | Yes | No | Yes | No | No | No |

**The problem:** Every existing tool solves one piece of the puzzle and needs 3-4 other tools to finish the job. Ansible handles provisioning but not dotfiles. Docker handles environments but not system packages. chezmoi handles dotfiles but not dependencies.

**Nexus closes the loop.** One binary. One `nexus init`. The security gate is always on. The orchestrator handles rollback. The dotfile engine handles push/pull. The WSL2 bridge handles Windows. You never touch a shell script.

---

## Security Model

Every command in Nexus passes through a security gate. No exceptions.

| Layer | Mechanism |
|---|---|
| **Command gate** | `SanitizeAndExecute` — allowlist only, no shell metacharacters, structured errors |
| **Input validation** | JSON Schema + Go struct validation for all profiles and configs |
| **Integrity** | SHA256 with constant-time comparison (`subtle.ConstantTimeCompare`) |
| **Network** | HTTPS-only. DNS-level SSRF blocking of private IPs. Host whitelist. 1 MB response limit |
| **State** | Atomic writes (temp file first, then rename). Append-only activity log |
| **WSL2** | Hardened `wsl.conf` with `noexec` automounts. No injection surface in setup scripts |
| **Profile names** | Regex `^[a-z0-9][a-z0-9-]*$`. Path traversal prevention via `filepath.Clean` + `filepath.EvalSymlinks` |
| **Config keys** | Allowlist validation. Rejects unknown keys |

See [SECURITY.md](SECURITY.md) for the full security policy and vulnerability reporting.

---

## Project Structure

```
nexus-engine/
├── cmd/
│   └── nexus/                 # CLI entry point
│       ├── main.go             # 17 cobra commands + viper wiring
│       ├── handlers.go         # 65 run functions (the command layer)
│       ├── main_test.go        # Integration tests
│       └── runner/             # Test runner subpackage
├── internal/
│   ├── bridge/                 # WSL2 detection (cross-compiled)
│   ├── container/              # Distrobox management
│   ├── dotfiles/               # chezmoi binding + push/pull
│   ├── engine/                 # Core: probe, execute, configure, state, audit
│   ├── installer/              # PackageManager (apt/pacman/dnf/apk) + orchestrator
│   ├── ledger/                 # Hardware compatibility ledger
│   ├── mode/                   # Operating mode switcher
│   ├── vault/                  # age-encrypted file store
│   └── wsl/                    # WSL2 rootfs import + downloader
├── pkg/
│   └── manifest/               # YAML profile schema + store + 10 embedded defaults
├── go.mod
├── Makefile
├── LICENSE
└── README.md
```

**Package breakdown:** 11 Go packages, 0 circular dependencies. Every package has its own test suite. Coverage ranges from 31.8% (CLI wiring) to 100% (WSL stubs).

---

## Test

```bash
# Run all tests
make test

# Run with race detector
make test-race

# Generate coverage report
make test-coverage

# Full quality gate
make check
```

**917 test functions** across **36 test files**. All 11 packages pass. Tests cover the orchestrator, command gate, WSL2 importer, dotfile push/pull, vault encrypt/decrypt, profile validation, teleport migration, and hardware ledger.

---

## Also Check Out

**[SubZero](https://github.com/Sumama-Jameel/subzero)** — Open source subscription tracker. Upload your bank statement as PDF, find every subscription, know what you're paying. Same philosophy: one action, zero data stored, open source.

---

## Build

```bash
# Prerequisites: Go 1.23+

# Build for current platform
make build

# Cross-compile all targets
make build-all
```

Produces a statically linked binary (`CGO_ENABLED=0`) with zero C dependencies.

---

## License

Licensed under [Apache License 2.0](LICENSE).

Copyright 2024-2026 Nexus Protocol Contributors.

See [NOTICE](NOTICE) for third-party attribution.