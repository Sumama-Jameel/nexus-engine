# Nexus Protocol

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/Sumama-Jameel/nexus-engine)](https://goreportcard.com/report/github.com/Sumama-Jameel/nexus-engine)
[![GoDoc](https://godoc.org/github.com/Sumama-Jameel/nexus-engine?status.svg)](https://godoc.org/github.com/Sumama-Jameel/nexus-engine)
[![CI Status](https://github.com/Sumama-Jameel/nexus-engine/actions/workflows/ci.yml/badge.svg)](https://github.com/Sumama-Jameel/nexus-engine/actions/workflows/ci.yml)

**Nexus Protocol — A Unified Computing Layer that bridges Windows and Linux with zero-friction, enterprise-grade automation.**

Nexus detects your operating system, probes your hardware, and automates a perfect developer environment. On Windows, it can import a Linux rootfs into WSL2 within 60 seconds. On Linux, it orchestrates package installation across apt, pacman, dnf, and apk. Declarative YAML profiles replace manual setup scripts. Every system call passes through a Zero-Trust security gate.

---

## Features

- **Zero-Trust Execution** — Every shell command passes through `SanitizeAndExecute` with an allowlisted command set, context timeouts, and structured error classification
- **60-Second WSL2 Setup** — `nexus wsl setup` downloads, imports, and configures a Linux environment inside Windows automatically
- **Declarative Profiles** — YAML manifests define your environment; compose with `extends`, validate in CI, share via the Community Ledger
- **SSRF-Safe Downloads** — HTTPS-only, DNS-level private IP rejection, host whitelisting, response size limits, schema validation before disk persistence
- **SHA256 Integrity Verification** — FIPS-compliant `crypto/sha256` with `subtle.ConstantTimeCompare` for all profile and rootfs integrity checks
- **Cross-Platform** — Static CGO_ENABLED=0 binaries for Linux (amd64/arm64) and Windows (amd64/arm64)
- **4 Package Managers** — apt, pacman, dnf, apk with a unified `PackageManager` interface and 6-step Orchestrator flow
- **Automatic Rollback** — Foundation package failures trigger automatic removal of all packages installed in the current run
- **Concurrent Installation** — Priority-ordered packages install concurrently within each group (Foundation → Language → Tool)
- **Audit Trail** — Append-only JSONL audit log with timestamps, actions, targets, and results
- **Atomic State** — `~/.nexus/state.json` with write-tmp-then-rename for crash safety and mutex-protected concurrency

---

## Quick Start

```bash
# 1. Install (choose one)
go install github.com/Sumama-Jameel/nexus-engine/cmd/nexus@latest
# — or download a binary from Releases

# 2. Make executable (if downloaded)
chmod +x nexus

# 3. Initialize your environment
nexus init
```

On Windows, the fastest path to Linux:

```bash
nexus wsl setup    # 60-second WSL2 setup — check, download, import, configure
nexus wsl enter    # Drop into your new Linux environment
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        NEXUS PROTOCOL                           │
│                                                                 │
│  ┌──────────┐   ┌──────────┐   ┌──────────┐                    │
│  │   BRAIN  │──▶│  BRIDGE  │──▶│   FACE   │                    │
│  │   (Go)   │   │  (WSL2)  │   │ (Tauri)  │                    │
│  │          │   │          │   │          │                    │
│  │ Cobra    │   │ Windows  │   │ Desktop  │                    │
│  │ Viper    │   │ ↕ Linux  │   │ GUI      │                    │
│  │ Engine   │   │ IPC      │   │          │                    │
│  └────┬─────┘   └────┬─────┘   └──────────┘                    │
│       │              │                                          │
│  ┌────▼─────┐   ┌────▼─────────────────────────┐               │
│  │   DNA    │   │        CONTAINER              │               │
│  │(Chezmoi) │   │      (Distrobox)              │               │
│  │          │   │                                │               │
│  │ Profiles │   │  Reproducible Linux           │               │
│  │ YAML     │   │  environments                 │               │
│  │ Extends  │   │                                │               │
│  └──────────┘   └────────────────────────────────┘               │
│                                                                 │
│  Security Layer: SanitizeAndExecute (Zero-Trust IPC)            │
│  Integrity Layer: SHA256 + JSON Schema + SSRF Protection        │
│  State Layer: Atomic JSON + Append-only JSONL Audit             │
└─────────────────────────────────────────────────────────────────┘
```

**The 5-Step Flow:** Probe → Validate → Apply → Configure → Report

**The 6-Step Orchestrator:** PreFlight → RefreshIndex → Order → Execute → Verify → Record

---

## Command Reference

| Command | Description |
|---|---|
| `nexus init` | Initialize the Nexus environment (probe, validate, apply, configure, report) |
| `nexus probe` | Probe the system — detect OS, hardware, and environment |
| `nexus version` | Print the Nexus Engine version |
| `nexus config get <key>` | Get a configuration value |
| `nexus install [packages...]` | Install packages via the Orchestrator |
| `nexus install --profile <name>` | Install packages from a named profile |
| `nexus remove [packages...]` | Remove Nexus-managed packages (with dependency warnings) |
| `nexus list` | List Nexus-managed packages and their verification status |
| `nexus search <query>` | Search for available packages |
| `nexus update [packages...]` | Update Nexus-managed packages |
| `nexus profile list` | List all available profiles with source and integrity hash |
| `nexus profile show <name>` | Show profile content, metadata, and resolved extends |
| `nexus profile validate <file>` | Validate a YAML profile against the Nexus Schema (CI-ready exit codes) |
| `nexus profile create <name>` | Create a new profile interactively via wizard |
| `nexus profile fetch <name>` | Fetch a profile from the remote Community Ledger |
| `nexus profile apply <name>` | Apply a profile — resolve and install via the Orchestrator |
| `nexus profile remove <name>` | Remove a profile from the local store |
| `nexus profile verify <name>` | Verify a profile's SHA256 integrity |
| `nexus wsl status` | Display full WSL2 detection report (Windows only) |
| `nexus wsl check` | Check if the system is ready for WSL2 setup (exit 0/1) |
| `nexus wsl import [image]` | Download and import a Linux rootfs into WSL2 |
| `nexus wsl setup` | One-command full WSL2 setup — the 60-second promise |
| `nexus wsl enter [distro]` | Enter a Nexus-managed WSL2 distribution |
| `nexus wsl remove [distro]` | Remove a Nexus-managed WSL2 distribution |
| `nexus wsl list` | List Nexus-managed WSL2 distributions |
| `nexus wsl images` | List available rootfs images for import |

Most commands support `--json` for structured output (IPC/API integration) and `--dry-run` where applicable.

---

## Security Model

Nexus operates on a **Zero-Trust execution model**. No shell command is ever executed directly.

| Layer | Mechanism |
|---|---|
| **Command Gate** | `SanitizeAndExecute` — allowlisted commands only, no shell metacharacters |
| **Input Validation** | JSON Schema + Go semantic validation for all profiles |
| **Integrity** | SHA256 with `subtle.ConstantTimeCompare` for profiles and rootfs images |
| **Network** | SSRF protection: HTTPS-only, DNS-level private IP rejection, host whitelist, 1MB response limit |
| **State** | Atomic writes (tmp → rename), append-only JSONL audit log |
| **WSL2** | Security-hardened `wsl.conf` (noexec automounts), embedded setup scripts (no injection surface) |
| **Profile Names** | Regex validation (`^[a-z0-9][a-z0-9-]*$`), path traversal prevention |

See [SECURITY.md](SECURITY.md) for vulnerability reporting and the full security policy.

---

## Project Structure

```
nexus-engine/
├── cmd/
│   └── nexus/
│       ├── main.go                # CLI entry point (Cobra commands)
│       └── runner/                # Business logic (Humble Object pattern)
│           ├── runner.go          # Dependencies struct, result types
│           ├── methods.go         # 8 command methods with DI
│           └── runner_test.go     # Unit tests with mock PackageManager
├── internal/
│   ├── bridge/                    # WSL2 detection & Windows↔Linux bridge
│   │   ├── bridge.go              # Environment detection core
│   │   ├── bridge_windows.go      # Windows-specific detection
│   │   ├── bridge_linux.go        # Linux-specific detection
│   │   ├── wsl_status.go          # WSL2 readiness model
│   │   └── wsl_status_linux.go    # Linux WSL2 stubs
│   ├── engine/                    # Core engine: probe, execute, configure
│   │   ├── probe.go               # System probe (OS, CPU, memory, GPU)
│   │   ├── execute.go             # SanitizeAndExecute (Zero-Trust gate)
│   │   ├── configure.go           # Shell & environment configuration
│   │   ├── config.go              # Viper configuration management
│   │   ├── state.go               # Atomic state tracker (~/.nexus/state.json)
│   │   └── audit.go               # Append-only JSONL audit logger
│   ├── installer/                 # Package management (The Orchestrator)
│   │   ├── installer.go           # PackageManager interface + factory
│   │   ├── apt.go                 # AptInstaller (Debian/Ubuntu)
│   │   ├── pacman.go              # PacmanInstaller (Arch/Manjaro)
│   │   ├── dnf.go                 # DnfInstaller (Fedora/RHEL)
│   │   ├── apk.go                 # ApkInstaller (Alpine)
│   │   ├── orchestrator.go        # 6-step Orchestrator with rollback
│   │   ├── preflight.go           # Pre-flight checks (disk, network, sudo, lock)
│   │   ├── verify.go              # Post-install binary verification
│   │   ├── preflight_linux.go     # Linux-specific pre-flight
│   │   └── preflight_windows.go   # Windows-specific pre-flight
│   └── wsl/                       # WSL2 import & management (The Bridge)
│       ├── rootfs.go              # RootFS registry & validation
│       ├── downloader.go          # SSRF-safe HTTP downloader
│       ├── import.go              # 7-step WSL2 importer
│       ├── wsl_windows.go         # Windows availability
│       └── wsl_linux.go           # Linux stubs (cross-compile)
├── pkg/
│   └── manifest/                  # Declarative YAML profiles (The DNA)
│       ├── manifest.go            # Two-layer validation (schema + Go)
│       ├── store.go               # Profile store with SHA256 integrity
│       ├── translator.go          # Target resolution & utilities
│       ├── defaults.go            # Embedded default profiles (go:embed)
│       ├── schemas/
│       │   └── nexus-profile.schema.json  # JSON Schema contract
│       └── defaults/
│           ├── base-dev.yaml      # Default developer profile
│           └── data-science.yaml  # Data science profile (extends base-dev)
├── go.mod
├── go.sum
├── Makefile
├── LICENSE
├── NOTICE
├── README.md
├── CONTRIBUTING.md
├── SECURITY.md
├── CHANGELOG.md
└── CODE_OF_CONDUCT.md
```

---

## Documentation

| Document | Description |
|---|---|
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute — setup, code style, PR process, DCO |
| [SECURITY.md](SECURITY.md) | Security policy — reporting, scope, safe harbor |
| [CHANGELOG.md](CHANGELOG.md) | Version history — all releases from v0.1.0 |
| [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) | Contributor Covenant v2.1 |
| [NOTICE](NOTICE) | Third-party attribution notices (Apache 2.0 requirement) |
| [GoDoc](https://godoc.org/github.com/Sumama-Jameel/nexus-engine) | API documentation for all exported symbols |

---

## Building from Source

```bash
# Prerequisites: Go 1.23+

# Build for current platform
make build

# Cross-compile all targets
make build-all

# Run tests
make test

# Run with race detector
make test-race

# Generate coverage report
make test-coverage

# Full quality gate (format, vet, test)
make check
```

The build produces a **statically linked binary** with zero C dependencies (`CGO_ENABLED=0`).

---

## License

This project is licensed under the [Apache License 2.0](LICENSE).

Copyright 2024-2026 Nexus Protocol Contributors.

See [NOTICE](NOTICE) for third-party attribution.
