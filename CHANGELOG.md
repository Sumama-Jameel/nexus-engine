# Changelog

All notable changes to the Nexus Protocol project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.6.0] - 2026-03-05

### Added
- Comprehensive test suite for all packages (engine, installer, bridge, wsl, manifest)
- GoDoc documentation on all exported symbols across all packages
- GitHub Actions CI/CD workflows (test, lint, build)
- GoReleaser configuration for automated multi-platform releases
- Apache License 2.0
- Professional project documentation (README, CONTRIBUTING, SECURITY, CHANGELOG)
- Code of Conduct (Contributor Covenant v2.1)
- Issue and PR templates for community contributions
- Dependabot configuration for dependency management
- `.gitignore` and repository hygiene

---

## [0.5.0] - 2026-02-26

### Added
- `internal/wsl/` package — new DDD bounded context for WSL2 import and management
- RootFS registry with hardcoded catalog: `nexus-alpine` (~3MB) and `nexus-debian` (~120MB)
- `ValidateDistroName()` — alphanumeric + hyphens validation, max 64 chars, no consecutive hyphens
- `ValidateInstallPath()` — path traversal prevention for WSL2 install paths
- `GenerateWSLConf()` — security-hardened wsl.conf with noexec automounts (prevents cross-OS malware)
- SSRF-safe HTTP downloader with DNS-level private IP rejection (RFC1918, loopback, link-local, IPv6 ULA)
- SHA256 verification with `crypto/subtle.ConstantTimeCompare` (prevents timing attacks)
- Atomic file writes for downloads (`.downloading` temp file → rename on completion)
- Response size limit of 1MB for remote downloads
- `WSL2Importer` with 7-step Import flow: PreFlight → Download → Configure → Import → Verify → ConfigurePost → Record
- Dependency-injected `ExecFunc` for Zero-Trust WSL command execution
- Embedded setup script avoids shell metacharacter injection
- `nexus wsl import [image]` command with `--name`, `--dry-run`, `--skip-verify`, `--skip-download` flags
- `nexus wsl setup` command — one-command 60-second WSL2 setup promise
- `nexus wsl remove [distro]` command with `--force` flag and Nexus-managed distro safety check
- `nexus wsl list` command — list Nexus-managed WSL2 instances from state
- `nexus wsl images` command — display rootfs image registry
- Linux stubs for cross-platform compilation (`wsl_linux.go`)
- `WSLInstanceState` and `WSLInstances` map in state tracker
- `RecordWSLImport()`, `RecordWSLRemove()`, `GetWSLInstances()`, `IsWSLManaged()` in state tracker
- `Target` field in `AuditEntry` for WSL operations
- Audit callback setup for all WSL operations (import, setup, remove)

### Fixed
- Alpine compatibility in setup script — detects and uses `adduser` (BusyBox) on Alpine, falls back to `useradd` on Debian/Fedora/Arch
- Double state recording in `runWSLImport` and `runWSLSetup` — removed duplicate explicit `state.RecordWSLImport()` calls (recording handled by callback)
- Unused global variables `remoteURL` and `wslImageName` removed

### Added (from gap audit)
- `nexus wsl enter [distro]` command — launch interactive shell inside a Nexus-managed WSL2 distribution
- Audit callback setup in `runWSLRemove` for complete audit coverage

---

## [0.4.0] - 2026-02-12

### Added
- WSL2 detection module in `internal/bridge/` with build-tag split for cross-platform compilation
- `WSL2Status` model — Windows version, WSL availability, installed distributions, Hyper-V status, readiness check
- `DetectEnvironment()` returns full WSL2 intelligence on Windows (`WSL2Status` field in `EnvironmentInfo`)
- `nexus wsl status` command — comprehensive WSL2 detection report
- `nexus wsl check` command — quick readiness check with correct exit codes (0=ready, 1=not ready) for CI/CD pipelines
- `ExecFunc` type in bridge package for Zero-Trust command execution
- `SetExecFunc()` for dependency injection of `SanitizeAndExecute` into bridge operations
- arm64 build targets in Makefile (`build-linux-arm64`, `build-windows-arm64`)
- `FormatEnvironmentInfo()` updated to display WSL2 status summary when present

### Fixed
- `nexus wsl check --json` now exits with code 1 when `ready: false` (previously returned exit 0 in JSON mode)
- `ValidatePrerequisites()` uses `where` on Windows instead of `which` for platform-appropriate binary lookup
- All 7 raw `exec.Command` calls in `bridge_windows.go` replaced with `bridgeExecFn` (Zero-Trust compliant)
- `detectEnvironmentImpl()` on Windows now stores WSL2 detection results instead of discarding them

---

## [0.3.0] - 2026-02-05

### Added
- Two-layer profile validation: JSON Schema (via gojsonschema) + Go semantic validation
- JSON Schema embedded in binary via `go:embed` (`pkg/manifest/schemas/`)
- `ProfileStore` with `~/.nexus/profiles/` + `registry.json` for local profile registry
- `ProfileMeta` tracking: name, version, source (bundled/local/remote), SHA256, date_added, last_applied
- `ProfileRegistry` with atomic writes (write tmp → rename) for crash safety
- SHA256 integrity verification using `crypto/sha256` (FIPS-compliant, zero deps)
- `VerifyIntegrity()` recomputes and compares hash on every profile load
- Remote profile fetching from GitHub Raw URLs (the Community Ledger)
- SSRF-safe fetching: constructed URLs from validated components only (no user-supplied URLs)
- Response size-limited to 1MB for remote profile downloads
- Schema-validated-before-disk-write (prevents persistence of malicious content)
- `nexus profile list` — show all profiles with source, version, SHA256, last applied
- `nexus profile show <name>` — display resolved profile with extends merged, metadata, env vars
- `nexus profile validate <file>` — validate YAML against schema with exit code 0/1 (CI-ready)
- `nexus profile create <name>` — interactive wizard generates valid YAML, validates, saves
- `nexus profile fetch <name>` — download from remote, validate, store with hash
- `nexus profile remove <name>` — remove from store (`--force` required for bundled)
- `nexus profile verify <name>` — SHA256 integrity check
- Environment variable injection from profiles into shell config
- `generateEnvExports()` with metacharacter sanitization
- Idempotent env var application via `NEXUS_START`/`NEXUS_END` marker replacement
- `NEXUS_PROFILE` env var always set to active profile name
- Profile composition via `extends` field
- `ResolveExtends()` with cycle detection (visited set), depth limit (MaxExtendsDepth=5)
- `mergeProfiles()` — additive package merge + child-wins env override
- `data-science.yaml` profile as extends demo (base-dev + ML packages)
- Bundled defaults auto-copied on first init

### Fixed (from gap audit)
- Removed `ApplyManifest` dead code that bypassed the Orchestrator (dangerous — skipped pre-flight, priority ordering, concurrent install, verify, rollback, audit, and state tracking)
- Removed `TranslateToCommand` (unused) and `InstallResult` (unused)
- Added `AllowedRemoteHosts` whitelist for remote profile fetching (`raw.githubusercontent.com`, `github.com`, `gist.githubusercontent.com`)
- Added `validateRemoteURL()` — HTTPS only, whitelisted hosts, no userinfo, no query params/fragments
- Added version drift detection on re-fetch — logs when remote version differs from local
- Added `nexus profile apply <name>` command — the declarative entry point for applying a profile
- Removed stale `configs/` directory (duplicates of embedded defaults)
- Fixed `ProfileMeta.Version` — now populated during store initialization by parsing profile content
- Single source of truth: `ProfileStore` is the canonical profile source; embedded defaults seed the store

---

## [0.2.0] - 2026-01-29

### Added
- `internal/installer/` package — new DDD bounded context for package management
- `PackageManager` interface with `Install`, `Remove`, `Update`, `IsInstalled`, `ListInstalled`, `Search`, `RefreshIndex` methods
- `AptInstaller` — per-package Install/Remove/Update/IsInstalled/ListInstalled/Search + `classifyAptError`
- `PacmanInstaller` — full CRUD + `classifyPacmanError`
- `DnfInstaller` — rpm-based checks + `classifyDnfError`
- `ApkInstaller` — no sudo, root-based + `classifyApkError`
- `PreFlightChecker` — 5 checks: disk space, network connectivity, sudo/root, package lock, already-installed
- `PostInstallVerifier` — binary-level verification for critical packages
- `Orchestrator` — 6-step flow (PreFlight → RefreshIndex → Order → Execute → Verify → Record)
- 3 inviolable Orchestrator rules: Foundation must succeed, priority ordering (Foundation → Language → Tool), verify after install
- `StateTracker` with `~/.nexus/state.json`, atomic writes (tmp → rename), `sync.Mutex` concurrency protection
- `AuditLogger` with `~/.nexus/audit.log`, `O_APPEND` append-only, JSONL format
- `nexus install [packages...]` command with `--profile`, `--dry-run` flags
- `nexus remove [packages...]` command with dependency-aware warnings
- `nexus list` command — show Nexus-managed packages with verification status
- `nexus search <query>` command — search available packages
- `nexus update [packages...]` command — update managed packages with index refresh
- `SanitizeAndExecute` whitelist expanded: added `apt-cache`, `dpkg`, `rpm`, `node`, `npm`, `python3`, `java`, `vim`, `curl`, `wget`, `zsh`, `htop`, `tmux`

### Fixed (from gap audit)
- `verifyBinary` panic on non-apt package managers — replaced hard type assertion with dependency-injected `ExecFunc`
- Added `RefreshIndex()` method to `PackageManager` interface — `apt-get update`, `pacman -Sy`, `dnf makecache`, `apk update` (prevents "Unable to locate package" on fresh systems)
- Added rollback mechanism — `RollbackReport` + `rollback()` in Orchestrator; foundation failures trigger automatic removal of all packages installed in current run
- Fixed pre-flight checks: `checkNetwork` now calls `pm.RefreshIndex()` (actually reaches repositories), `checkSudo` now runs `sudo -n true`, `checkLock` now uses `syscall.Flock` with `LOCK_EX|LOCK_NB`
- Added dependency-aware removal: `buildDependencyMap()` warns when removing packages that other managed packages depend on
- Added concurrent installation within priority groups via `installGroup()` with goroutines and channel-based result collection
- Pre-flight results now displayed in `FormatOrchestratorResult`
- Separated `initConfigPath` and `profilePath` to avoid confusion between `init --config` and `install --profile`

---

## [0.1.0] - 2026-01-15

### Added
- Go module initialized at `github.com/Sumama-Jameel/nexus-engine` with Go 1.23.6
- Dependencies: Cobra v1.10.2 (CLI), Viper v1.21.0 (configuration), yaml.v3
- Strict directory structure: `cmd/nexus`, `internal/engine`, `internal/bridge`, `pkg/manifest`, `configs`
- `internal/engine/probe.go` — system probe (OS, CPU, memory, GPU)
- `internal/engine/execute.go` — `SanitizeAndExecute` Zero-Trust command gate with allowlisted commands and 60-second context timeouts
- `internal/engine/configure.go` — shell and environment configuration
- `internal/engine/config.go` — Viper-based configuration management
- `internal/bridge/bridge.go` — OS and environment detection
- `pkg/manifest/manifest.go` — YAML profile parsing and validation
- `pkg/manifest/translator.go` — manifest-to-command translation
- `configs/base-dev.yaml` — default developer profile with high-utility tools
- `configs/nexus-profile.schema.json` — JSON Schema for Nexus Profile YAML
- `cmd/nexus/main.go` — Cobra CLI entry point
- `nexus probe` command — detect OS, hardware, and environment
- `nexus init` command — full 5-step initialization (Probe → Validate → Apply → Configure → Report) with `--dry-run`, `--json`, `--config` flags
- `nexus version` command — print engine version
- `nexus config get <key>` command — get configuration values
- Statically linked binary with `CGO_ENABLED=0` (zero C dependencies)

---

[0.6.0]: https://github.com/Sumama-Jameel/nexus-engine/releases/tag/v0.6.0
[0.5.0]: https://github.com/Sumama-Jameel/nexus-engine/releases/tag/v0.5.0
[0.4.0]: https://github.com/Sumama-Jameel/nexus-engine/releases/tag/v0.4.0
[0.3.0]: https://github.com/Sumama-Jameel/nexus-engine/releases/tag/v0.3.0
[0.2.0]: https://github.com/Sumama-Jameel/nexus-engine/releases/tag/v0.2.0
[0.1.0]: https://github.com/Sumama-Jameel/nexus-engine/releases/tag/v0.1.0
