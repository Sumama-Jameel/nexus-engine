# Nexus Protocol

[![Go Version](https://img.shields.io/badge/Go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/Sumama-Jameel/nexus-engine)](https://goreportcard.com/report/github.com/Sumama-Jameel/nexus-engine)
[![GoDoc](https://godoc.org/github.com/Sumama-Jameel/nexus-engine?status.svg)](https://godoc.org/github.com/Sumama-Jameel/nexus-engine)

**Nexus Protocol. One environment. One command. Any machine.**

---

## What This Solves

Setting up a development machine from scratch takes hours. You run different commands on Ubuntu versus Arch versus Fedora. Your dotfiles handle shell config but not dependencies. Distrobox needs a working Linux install first. Ansible is built for server fleets, not one laptop.

Nexus is one static binary that does all of it. It detects your operating system, probes your hardware, picks the right package manager (apt, pacman, dnf, apk), installs the software you need from a declarative YAML profile, manages your dotfiles through chezmoi, and on Windows it can import a Linux environment into WSL2 in 60 seconds. No scripts to maintain. No manual setup steps. One command to go from fresh OS to productive environment.

You can write your own profile, pick from 8 built-in profiles (developer, gamer, pentesting, productivity), pull profiles from the community registry, or let Nexus suggest one based on your hardware and distro.

---

## What We Need

Yeah you.

Nexus is just a binary. What makes it hit is the squad behind it. We need you to drop profiles, tweak manifests, and tell your people about it. Got a sick dev setup? Wrap it as a profile and toss it in the registry. Running something jank? Open an issue. Windows user tired of fighting your own machine? Nexus is your way in. One binary, one command, and you are coding on Linux before the coffee brews.

Linux grows when we make it easy. Not just for the graybeards. For everyone. The Windows kid who wants to learn. The gamer who needs Steam plus Vulkan without the headache. The homie who just wants apt to work and go home.

So here is the play:
- Fork the repo.
- Add your profile or fix someone else's.
- Push it up.
- Tell a friend.

That is it. That is the whole thing.

---

## Quick Start

```bash
# Install (choose one)
go install github.com/Sumama-Jameel/nexus-engine/cmd/nexus@latest
# -- or download a binary from https://github.com/Sumama-Jameel/nexus-engine/releases

# Initialize your environment
nexus init
```

On Windows, get Linux in one command:

```bash
nexus wsl setup    # download, import, and configure WSL2
nexus wsl enter    # jump into your new Linux environment
```

---

## Features

- **Sandboxed command execution.** All shell commands pass through a security gate with an allowlist, timeouts, and structured error handling.
- **Declarative YAML profiles.** Write a profile that declares what your environment needs. Compose profiles with `extends`. Share them with your team or pull from the community.
- **Distro-aware suggestions.** Profiles can declare a target distro. Nexus checks your OS, warns on mismatches, and suggests WSL2 or Distrobox as a fallback. 8 profiles ship with the binary.
- **Four package managers.** apt, pacman, dnf, and apk behind a single interface. Write a profile once, apply it on any distro.
- **Parallel package installation.** Packages install concurrently within priority groups (Foundation, Language, Tool).
- **Automatic rollback.** If a foundation package fails to install, Nexus removes everything it installed in that run to leave your system clean.
- **Secure downloads.** HTTPS only. DNS-level blocking of private IP addresses. Host whitelisting. Response size limits. Schema validation before anything touches disk.
- **Integrity verification.** SHA256 with constant-time comparison for every profile and rootfs image.
- **Set up WSL2 in 60 seconds.** `nexus wsl setup` checks readiness, downloads a rootfs, imports it into WSL2, and writes a hardened wsl.conf.
- **Dotfile management.** Bind a Git repository through chezmoi. Push and pull with pre-push secret scanning and ff-only pull by default.
- **age-encrypted vault.** Encrypt sensitive files with age. Keys never leave your machine.
- **Operating modes.** Switch between base, development, hardened, container, and gaming modes. Each mode adjusts settings and available commands.
- **Distrobox containers.** Create, enter, and delete isolated Linux environments.
- **Community registry.** List, search, fetch, and submit profiles through a global registry.
- **Hardware compatibility ledger.** Record and query hardware reports. Check if your hardware is known to work with Nexus.
- **Windows-to-WSL2 folder migration.** Symlink your Windows Documents, Desktop, Downloads, and Pictures into WSL2 with one command.
- **Crash-safe state.** State files are written atomically (write to temp file, then rename). Appends to an activity log you can audit.
- **Statically linked binary.** CGO_ENABLED=0. Zero C dependencies. Works on any Linux (amd64 or arm64) and Windows.
- **Desktop GUI (optional).** A Tauri dashboard that shows all system state and can run commands through the Go engine.

---

## Architecture

```
+----------------------------------------------------------------------+
|                          NEXUS PROTOCOL                              |
|                                                                      |
|  +-------------+    +-------------+    +-------------+               |
|  |   BRAIN     |---?|   BRIDGE    |---?|    FACE     |               |
|  |   (Go)      |    |   (WSL2)    |    |  (Tauri)    |               |
|  |             |    |             |    |             |               |
|  | Cobra       |    | Windows     |    | Desktop     |               |
|  | Viper       |    | ? Linux     |    | GUI         |               |
|  | Engine      |    | IPC         |    |             |               |
|  | 16 built-in |    |             |    | 7 tabs      |               |
|  | profiles    |    |             |    |             |               |
|  +------+------+    +------+------+    +-------------+               |
|         |                   |                                        |
|  +------v------+    +------v------------------------+                |
|  |     DNA     |    |          CONTAINER             |               |
|  |  (Profiles) |    |        (Distrobox)             |               |
|  |             |    |                                |               |
|  | YAML schema |    |  Reproducible environments     |               |
|  | Validation  |    |  for any distro                |               |
|  | SHA256 hash |    |                                |               |
|  +-------------+    +--------------------------------+               |
|                                                                      |
|  Security layer: SanitizeAndExecute (command gate)                   |
|  Integrity layer: SHA256 + JSON Schema + network guards              |
|  State layer: Atomic JSON + append-only log                          |
+----------------------------------------------------------------------+
```

**Bounded contexts:**

- **BRAIN** (Go engine). Cobra CLI, Viper configuration, engine core, dotfiles, vault, mode switcher, profile suggestions, hardware ledger.
- **DNA** (YAML profiles). Declarative manifests, 8 built-in profiles, JSON Schema validation, SHA256 integrity, registry bridge.
- **BRIDGE** (WSL2). Windows-to-Linux IPC, 60-second WSL2 setup, Windows folder migration.
- **VAULT** (V9). age-encrypted file store with key management.
- **FACE** (V10, Tauri). Desktop GUI with 7 tabs (Sync, Vault, System, Profile, Modes, Containers, Registry).
- **MODES** (V11). Operating mode switching with per-mode configuration.
- **CONTAINER** (V12). Distrobox container management (create, enter, delete, list).
- **LEDGER** (V13). Hardware compatibility ledger with privacy-first fingerprinting.
- **TELEPORT** (V14). Windows-to-WSL2 user-folder symlink migration.
- **REGISTRY** (V15). Global profile marketplace with search, fetch, and submit.
- **SUGGEST** (V16). Distro-aware profile recommendations with WSL2 and Distrobox fallback.

**The 5-step flow:** Probe. Validate. Apply. Configure. Report.

**The 7-step orchestrator:** PreFlight. RefreshIndex. Order. Execute. Verify. Record. Audit.

---

## Command Reference

| Command | Description |
|---|---|
| `nexus init` | Initialize the Nexus environment (probe, validate, apply, configure, report) |
| `nexus probe` | Detect OS, hardware, distro, and environment |
| `nexus version` | Print the Nexus Engine version |
| `nexus config get <key>` | Get a configuration value |
| `nexus install [packages...]` | Install packages through the Orchestrator |
| `nexus install --profile <name>` | Install packages from a named profile |
| `nexus remove [packages...]` | Remove Nexus-managed packages (with dependency warnings) |
| `nexus list` | List Nexus-managed packages and verification status |
| `nexus search <query>` | Search for available packages |
| `nexus update [packages...]` | Update Nexus-managed packages |
| `nexus profile list` | List all profiles with source and integrity hash |
| `nexus profile show <name>` | Show profile content, metadata, and resolved extends |
| `nexus profile validate <file>` | Validate a YAML profile against the Nexus Schema |
| `nexus profile create <name>` | Create a new profile interactively through a wizard |
| `nexus profile fetch <name>` | Fetch a profile from the remote Registry |
| `nexus profile apply <name>` | Apply a profile (resolve and install through the Orchestrator) |
| `nexus profile remove <name>` | Remove a profile from the local store |
| `nexus profile verify <name>` | Verify a profile's SHA256 integrity |
| `nexus profile suggest` | Suggest profiles compatible with the current system |
| `nexus vault init` | Initialize the Nexus vault (age key) |
| `nexus vault status` | Show vault status and key fingerprint |
| `nexus vault list` | List vault-encrypted files |
| `nexus vault encrypt <file>` | Encrypt a file into the vault |
| `nexus vault decrypt <name>` | Decrypt and restore a vault file |
| `nexus mode list` | List available operating modes |
| `nexus mode get` | Show the current operating mode |
| `nexus mode set <mode>` | Switch operating mode (base, development, hardened, container, gaming) |
| `nexus container list` | List running and available containers |
| `nexus container create <name>` | Create a new Distrobox container |
| `nexus container enter <name>` | Enter an existing container |
| `nexus container delete <name>` | Remove a container |
| `nexus wsl status` | Display full WSL2 detection report (Windows only) |
| `nexus wsl check` | Check if the system is ready for WSL2 setup |
| `nexus wsl import [image]` | Download and import a Linux rootfs into WSL2 |
| `nexus wsl setup` | Full WSL2 setup in one command |
| `nexus wsl enter [distro]` | Enter a Nexus-managed WSL2 distribution |
| `nexus wsl remove [distro]` | Remove a Nexus-managed WSL2 distribution |
| `nexus wsl list` | List Nexus-managed WSL2 distributions |
| `nexus wsl images` | List available rootfs images for import |

### V7. Dotfile Management (The Memory)

| Command | Description |
|---|---|
| `nexus dotfiles detect` | Check whether chezmoi is installed and report version |
| `nexus dotfiles install` | Install chezmoi through the system package manager |
| `nexus dotfiles init <repo-url>` | Bind a Git dotfile repository (HTTPS only, host-whitelisted) |
| `nexus dotfiles remove` | Unbind the dotfile source from Nexus state |
| `nexus dotfiles apply` | Apply managed dotfiles from the bound source |
| `nexus dotfiles status` | Show the current chezmoi state |
| `nexus dotfiles diff` | Show pending changes between source and live system |
| `nexus dotfiles add <path>` | Track an existing file with chezmoi (use `--force` for sensitive paths) |
| `nexus dotfiles verify` | Verify the live system matches the bound source |
| `nexus dotfiles push` | Stage, secret-scan, commit, and push local changes to the remote |
| `nexus dotfiles pull` | Fetch from remote and apply (uses `--ff-only` by default; `--rebase` to opt in) |
| `nexus dotfiles sync` | Pull, apply, and push in one shot |

### V8. Git-Sync Engine (The Cloud)

| Command | Description |
|---|---|
| `nexus dotfiles push` | Push local changes with pre-push secret scanning |
| `nexus dotfiles pull` | Pull remote changes with ff-only by default |
| `nexus dotfiles sync` | Pull + apply + push in a single command |

All commands support `--json` for structured output and `--dry-run` to preview without making changes.

### V13. Hardware Ledger (The Intelligence)

| Command | Description |
|---|---|
| `nexus ledger record` | Record a hardware report from the current system |
| `nexus ledger status` | Show ledger statistics and sync status |
| `nexus ledger query <field> <value>` | Query the ledger for hardware compatibility data |
| `nexus ledger check` | Check if your hardware is known to work with Nexus |
| `nexus ledger sync` | Push and pull records to and from the community ledger |
| `nexus ledger pull` | Download community compatibility data for offline matching |

### V14. Teleport Migration (The Closer)

| Command | Description |
|---|---|
| `nexus teleport` | Symlink Windows Documents, Desktop, Downloads, and Pictures into WSL2 |
| `nexus teleport --dry-run` | Preview what would be linked without making changes |

---

## Security Model

Nexus uses a sandboxed execution model. No shell command runs directly.

| Layer | Mechanism |
|---|---|
| **Command gate** | `SanitizeAndExecute` only allows listed commands. No shell metacharacters. |
| **Input validation** | JSON Schema and Go semantic validation for all profiles. |
| **Integrity** | SHA256 with constant-time comparison for profiles and rootfs images. |
| **Network** | HTTPS-only downloads. DNS-level blocking of private IPs. Host whitelist. 1 MB response limit. |
| **State** | Atomic writes (temp file, then rename). Append-only activity log. |
| **WSL2** | Hardened wsl.conf with noexec automounts. Embedded setup scripts with no injection surface. |
| **Profile names** | Regex validation (`^[a-z0-9][a-z0-9-]*$`). Path traversal prevention. |

See [SECURITY.md](SECURITY.md) for vulnerability reporting and the full security policy.

---

## Project Structure

```
nexus-engine/
+-- cmd/
|   +-- nexus/
|       +-- main.go                # CLI entry point (Cobra commands)
+-- internal/
|   +-- bridge/                    # WSL2 detection and Windows-Linux bridge
|   |   +-- bridge.go              # Environment detection core
|   |   +-- bridge_windows.go      # Windows-specific detection
|   |   +-- bridge_linux.go        # Linux-specific detection
|   |   +-- wsl_status.go          # WSL2 readiness model
|   |   +-- wsl_status_linux.go    # Linux WSL2 stubs
|   +-- engine/                    # Core engine: probe, execute, configure
|   |   +-- probe.go               # System probe (OS, CPU, memory, GPU, distro)
|   |   +-- execute.go             # SanitizeAndExecute (command gate)
|   |   +-- configure.go           # Shell and environment configuration
|   |   +-- config.go              # Viper configuration management
|   |   +-- state.go               # Atomic state tracker
|   |   +-- audit.go               # Append-only activity log
|   |   +-- netguard.go            # Shared SSRF primitives
|   |   +-- profiles.go            # Distro-aware SuggestProfile and CheckDistroCompatibility
|   |   +-- profiles_test.go       # Tests for distro compatibility
|   +-- installer/                 # Package management (The Orchestrator)
|   |   +-- installer.go           # PackageManager interface and factory
|   |   +-- apt.go                 # AptInstaller (Debian, Ubuntu)
|   |   +-- pacman.go              # PacmanInstaller (Arch, Manjaro)
|   |   +-- dnf.go                 # DnfInstaller (Fedora, RHEL)
|   |   +-- apk.go                 # ApkInstaller (Alpine)
|   |   +-- orchestrator.go        # 7-step Orchestrator with rollback
|   |   +-- preflight.go           # Pre-flight checks (disk, network, sudo, lock)
|   |   +-- verify.go              # Post-install binary verification
|   |   +-- preflight_linux.go     # Linux-specific pre-flight
|   |   +-- preflight_windows.go   # Windows-specific pre-flight
|   +-- wsl/                       # WSL2 import and management (The Bridge)
|   |   +-- rootfs.go              # RootFS registry and validation
|   |   +-- downloader.go          # SSRF-safe HTTP downloader
|   |   +-- import.go              # 7-step WSL2 importer
|   |   +-- wsl_windows.go         # Windows availability
|   |   +-- wsl_linux.go           # Linux stubs (cross-compile)
|   +-- dotfiles/                  # V7: Dotfile management (The Memory)
|   |   +-- types.go               # DetectReport and error types
|   |   +-- detect.go              # Probe chezmoi installation
|   |   +-- install.go             # Install through Orchestrator
|   |   +-- source.go              # Bind and unbind Git source (SSRF-safe)
|   |   +-- apply.go               # Apply, Status, Diff
|   |   +-- track.go               # Add (with path validation), Verify
|   |   +-- profile.go             # ApplyFromProfile bridge
|   |   +-- dotfiles_linux.go      # Linux path resolution
|   |   +-- dotfiles_windows.go    # Windows path resolution
|   |   +-- push.go                # V8: Secret-scan, commit, push
|   |   +-- pull.go                # V8: Pull with ff-only or rebase
|   +-- vault/                     # V9: age-encrypted file vault
|   |   +-- vault.go               # Vault init, encrypt, decrypt, list
|   |   +-- vault_test.go
|   +-- mode/                      # V11: Operating mode switcher
|   |   +-- mode.go                # Mode management
|   |   +-- mode_test.go
|   +-- container/                 # V12: Distrobox container management
|   |   +-- container.go           # Container create, enter, delete, list
|   |   +-- container_test.go
|   +-- ledger/                    # V13: Hardware compatibility ledger
|   |   +-- ledger.go              # Record, query, sync
|   |   +-- fingerprint.go         # Privacy-first hardware fingerprinting
|   |   +-- ledger_test.go
|   +-- teleport/                  # V14: Windows-to-WSL2 folder migration
|       +-- teleport.go            # Symlink Documents, Desktop, Downloads
|       +-- teleport_test.go
+-- pkg/
|   +-- manifest/                  # Declarative YAML profiles (The DNA)
|       +-- manifest.go            # Two-layer validation (schema + Go)
|       +-- store.go               # Profile store with SHA256 integrity
|       +-- translator.go          # Target resolution and utilities
|       +-- defaults.go            # Embedded default profiles (go:embed)
|       +-- schemas/
|       |   +-- nexus-profile.schema.json     # JSON Schema contract
|       +-- defaults/
|           +-- base-dev.yaml
|           +-- data-science.yaml
|           +-- base-gamer.yaml          # V16: Gaming environment
|           +-- base-gamer-nvidia.yaml   # V16: NVIDIA GPU gaming
|           +-- base-gamer-amd.yaml      # V16: AMD GPU gaming + ROCm
|           +-- base-work.yaml           # V16: Office and productivity
|           +-- ethical-hacker.yaml      # V16: Pentesting toolkit
|           +-- frontend-dev.yaml        # V16: Node + TypeScript
|           +-- rust-dev.yaml            # V16: Rust toolchain
|           +-- go-dev.yaml              # V16: Go toolchain
+-- go.mod
+-- go.sum
+-- Makefile
+-- LICENSE
+-- NOTICE
+-- README.md
+-- CONTRIBUTING.md
+-- SECURITY.md
+-- CHANGELOG.md
+-- CODE_OF_CONDUCT.md
```

---

## Documentation

| Document | Description |
|---|---|
| [CONTRIBUTING.md](CONTRIBUTING.md) | How to contribute. Setup, code style, PR process, DCO. |
| [SECURITY.md](SECURITY.md) | Security policy. Reporting, scope, safe harbor. |
| [CHANGELOG.md](CHANGELOG.md) | Version history from v0.1.0. |
| [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) | Contributor Covenant v2.1. |
| [NOTICE](NOTICE) | Third-party attribution notices (Apache 2.0 requirement). |
| [GoDoc](https://godoc.org/github.com/Sumama-Jameel/nexus-engine) | API documentation for all exported symbols. |

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

The build produces a statically linked binary with zero C dependencies (CGO_ENABLED=0).

---

## License

This project is licensed under the [Apache License 2.0](LICENSE).

Copyright 2024-2026 Nexus Protocol Contributors.

See [NOTICE](NOTICE) for third-party attribution.
