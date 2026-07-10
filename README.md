# Nexus Protocol

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/Sumama-Jameel/nexus-engine)](https://goreportcard.com/report/github.com/Sumama-Jameel/nexus-engine)
[![GoDoc](https://godoc.org/github.com/Sumama-Jameel/nexus-engine?status.svg)](https://godoc.org/github.com/Sumama-Jameel/nexus-engine)
[![Tests](https://img.shields.io/badge/Tests-917%20passing-brightgreen)](https://go.dev/)

**Open source alternative to Ansible, Distrobox, and shell scripts. One static binary. Any machine.**

---

## What This Solves

Only 7% of organizations can set up a development machine in under an hour. 21% take more than two days. (Coder 2025 survey of 550+ software professionals.)

It gets worse:

- Over 600 active Linux distributions exist. Each one uses a different package manager. apt on Ubuntu. pacman on Arch. dnf on Fedora. apk on Alpine. Your scripts work on one and break on the rest.
- Developers lose 3 hours every week to tool failures and setup problems. That is 20 workdays per year. (Lokalise 2025 survey of 500 US developers.)
- 39% of developers say tool setup is a major time sink. (Atlassian 2025 survey of 3,500 respondents.)

Most people turn to shell scripts, Docker, or Ansible to fix this. Each tool has a problem:

| Tool | Problem |
|---|---|
| Shell scripts | One apt failure in a 200-line script and the whole thing breaks. No rollback. No state. No security. |
| Docker | Needs a daemon. Needs sudo. Needs a running container. Zero help with your actual host machine. |
| Ansible | Built for server fleets, not one laptop. Playbooks expect SSH access to every target. |
| Distrobox | Needs a working Linux install first. You need a working machine to set up a working machine. |
| Dotfile managers | Great for config files. Useless for dependencies. Your dotfiles do not install gcc. |

That is broken.

**Nexus is different.** One static Go binary (zero C dependencies, works on any Linux and Windows) detects your OS, probes your hardware, picks the right package manager, and installs everything from a YAML profile. No scripts. No manual steps. A 7-step security gate wraps every command. If a foundation package fails, Nexus rolls back everything it installed.

And it is open source. Not a SaaS subscription to set up your own machine.

---

## What We Need

You.

Nexus is open source because the Linux desktop deserves a setup tool that works for everyone. Not just for people who have memorized the pacman flags.

Here is how you help:

- **Drop a profile.** Got a good dev setup? Wrap it as a YAML profile and put it in the registry. 10 profiles ship with the binary. Add yours.
- **Open an issue.** Running something jank? Something broke on your distro? Tell us. Every fix makes Nexus work for one more person.
- **Help a Windows kid migrate.** Windows users who want Linux should not need a CS degree to get there. Nexus makes it one command: `nexus wsl setup`. Tell them about it.

That's it. That's the whole thing.

---

## Quick Start

```bash
# Install (choose one)
go install github.com/Sumama-Jameel/nexus-engine/cmd/nexus@latest
# or download a binary from https://github.com/Sumama-Jameel/nexus-engine/releases

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

- **4 package managers, one interface.** apt, pacman, dnf, apk behind a single command. Write a profile once. Run it on Ubuntu, Arch, Fedora, or Alpine. No changes needed.
- **7-step orchestrator with rollback.** PreFlight. RefreshIndex. Order. Execute. Verify. Record. Audit. If a foundation package fails, Nexus removes everything from that run. No partial installs.
- **Security-gated execution.** Every command passes through SanitizeAndExecute. An allowlist. Metacharacter rejection. Timeouts. Structured errors. No command runs directly on your shell.
- **10 built-in profiles.** Go developer. Rust developer. Frontend developer. Data scientist. Gamer. Ethical hacker. Productivity. Each profile is a curated YAML file embedded in the binary.
- **Distro-aware suggestions.** Profiles can declare a target distro. Nexus checks your OS, warns on mismatches, and suggests WSL2 or Distrobox as a fallback.
- **Parallel package installs.** Packages install concurrently within priority groups (Foundation, Language, Tool). No serial apt-get chains.
- **Secure downloads.** HTTPS only. DNS-level SSRF blocking of private IPs. Host whitelisting. 1 MB response limits. Schema validation before anything touches disk.
- **SHA256 integrity.** Constant-time comparison for every profile and rootfs image. No timing side channels.
- **WSL2 setup in 60 seconds.** nexus wsl setup checks readiness, downloads a rootfs, imports it into WSL2, and writes a hardened wsl.conf with noexec automounts.
- **Dotfile management through chezmoi.** Bind a Git repo. Push with pre-push secret scanning. Pull with ff-only by default. nexus dotfiles push stages, scans, and commits in one shot.
- **age-encrypted vault.** Encrypt sensitive files with age. Keys never leave your machine. nexus vault encrypt and nexus vault decrypt through the same interface.
- **5 operating modes.** Base. Development. Hardened. Container. Gaming. Each mode adjusts settings and available commands.
- **Distrobox containers.** Create, enter, delete, and list isolated Linux environments from the same binary.
- **Community registry.** List, search, fetch, and submit profiles. nexus profile fetch and nexus profile submit from the CLI.
- **Hardware compatibility ledger.** Record, query, and sync hardware reports. Check if your hardware is known to work with Nexus.
- **Windows to WSL2 folder migration.** Symlink your Windows Documents, Desktop, Downloads, and Pictures into WSL2 with nexus teleport.
- **Crash-safe state.** Atomic writes (write to temp file, then rename). Append-only activity log. State files survive kill -9.
- **Desktop GUI (optional).** A Tauri dashboard with 7 tabs (Sync, Vault, System, Profile, Modes, Containers, Registry) that talks to the Go engine.

---

## Architecture

```
                     +---------------------------------------------+
                     |              NEXUS PROTOCOL                  |
                     |                                               |
                     |  +-----------+    +-----------+               |
                     |  |   BRAIN   |----|   BRIDGE   |              |
                     |  |  (Go)     |    |  (WSL2)   |              |
                     |  |           |    |           |              |
                     |  | Cobra    |    | Windows   |              |
                     |  | Viper    |    | to Linux  |              |
                     |  | Engine   |    | IPC       |              |
                     |  | 10       |    |           |              |
                     |  | profiles  |    |           |              |
                     |  +-----+-----+    +-----+-----+              |
                     |        |                 |                    |
                     |  +-----v-----+   +-----v-----------+         |
                     |  |   DNA     |   |   CONTAINER     |         |
                     |  | (YAML)    |   |  (Distrobox)    |         |
                     |  |           |   |                 |         |
                     |  | SHA256    |   | Reproducible    |         |
                     |  | Schema    |   | environments    |         |
                     |  | Validation|   |                 |         |
                     |  +-----------+   +-----------------+         |
                     |                                               |
                     |  Security: SanitizeAndExecute (command gate)  |
                     |  Integrity: SHA256 + JSON Schema + netguard    |
                     |  State: Atomic JSON + append-only log          |
                     +---------------------------------------------+
```

**Bounded contexts:**

- **BRAIN** - Cobra CLI, Viper config, Go engine. All commands route through here. 17 top-level commands. The SanitizeAndExecute command gate lives here.
- **DNA** - Declarative YAML profiles. Two-layer validation: JSON Schema first, Go struct validation second. 10 embedded profiles, SHA256 indexed.
- **BRIDGE** - WSL2 detection and Windows to Linux bridge. Cross compiled for Linux and Windows.
- **CONTAINER** - Distrobox management. Decoupled from the engine via the StateTracker interface.
- **VAULT** - age-encrypted file store. Keys stay on your machine.
- **REGISTRY** - Community profile marketplace. Search, fetch, submit.

**The 5-step flow:** Probe. Validate. Apply. Configure. Report.

**The 7-step orchestrator:** PreFlight. RefreshIndex. Order. Execute. Verify. Record. Audit.

---

## Competitor Comparison

Nexus is the open source alternative to every tool on this list.

| Tool | Static binary | Cross distro | Security gate | Auto rollback | Dotfiles | WSL2 | 1 command init |
|---|---|---|---|---|---|---|---|
| **Nexus** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| Ansible | No | Yes | No | No | No | No | No |
| Distrobox | No | Yes | No | No | No | No | No |
| Docker | No | Yes | No | No | No | No | No |
| chezmoi | Yes | Yes | No | No | Yes | No | No |
| dotbot | No | No | No | No | Yes | No | No |
| Shell script | No | No | No | No | No | No | No |
| Nix | No | Yes | No | Yes | No | No | No |

Every existing tool solves one piece and needs 3-4 other tools to finish the job. Ansible handles provisioning but not dotfiles. Docker handles environments but not system packages. chezmoi handles dotfiles but not dependencies.

Nexus closes the loop. One binary. One `nexus init`. The security gate is always on. The orchestrator handles rollback. The dotfile engine handles push and pull. The WSL2 bridge handles Windows. You never touch a shell script.

---

## Security Model

Every command in Nexus passes through a security gate. No exceptions.

| Layer | Mechanism |
|---|---|
| **Command gate** | SanitizeAndExecute. Allowlist only. No shell metacharacters. Structured errors. |
| **Input validation** | JSON Schema + Go struct validation for all profiles and configs. |
| **Integrity** | SHA256 with constant-time comparison (subtle.ConstantTimeCompare). |
| **Network** | HTTPS only. DNS-level SSRF blocking of private IPs. Host whitelist. 1 MB response limit. |
| **State** | Atomic writes (temp file first, then rename). Append-only activity log. |
| **WSL2** | Hardened wsl.conf with noexec automounts. No injection surface in setup scripts. |
| **Profile names** | Regex `^[a-z0-9][a-z0-9-]*$`. Path traversal prevention via filepath.Clean + filepath.EvalSymlinks. |
| **Config keys** | Allowlist validation. Rejects unknown keys. |

See [SECURITY.md](SECURITY.md) for the full security policy.

---

## Project Structure

```
nexus-engine/
├── cmd/nexus/             # CLI entry point (cobra commands, handlers)
├── internal/
│   ├── bridge/            # WSL2 detection (cross compiled)
│   ├── container/         # Distrobox management
│   ├── dotfiles/          # chezmoi binding and push/pull
│   ├── engine/            # Core: probe, execute, configure, state, audit
│   ├── installer/         # Package manager (apt/pacman/dnf/apk) + orchestrator
│   ├── ledger/            # Hardware compatibility ledger
│   ├── mode/              # Operating mode switcher
│   ├── vault/             # age-encrypted file store
│   └── wsl/               # WSL2 rootfs import and downloader
├── pkg/manifest/          # YAML profile schema, store, 10 embedded defaults
├── go.mod
├── Makefile
└── LICENSE
```

---

## Test

```bash
make test
```

All 11 packages pass.

```bash
make test-race    # run with race detector
make check        # full quality gate (format, vet, test)
```

---

## Build

```bash
# Prerequisites: Go 1.23+

make build              # build for current platform
make build-all          # cross compile all targets
```

Produces a statically linked binary with zero C dependencies (CGO_ENABLED=0).

---

## Also Check Out

**[SubZero](https://github.com/Sumama-Jameel/subzero)** - Open source subscription tracker. Upload your bank statement as PDF. Find every subscription. Know what you are paying. Same philosophy: one action, zero data stored, open source.

---

## License

Licensed under [Apache License 2.0](LICENSE).

Copyright 2024-2026 Nexus Protocol Contributors.

See [NOTICE](NOTICE) for third-party attribution.