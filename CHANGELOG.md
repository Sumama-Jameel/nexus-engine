# Changelog

All notable changes to the Nexus Protocol project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.15.0] - 2026-07-09

### Added
- V15: The Global Registry (The Launch) — community profile discovery
- `internal/engine/registry.go` — `RegistryProfile`, `ListRegistry`, `SearchRegistry`, `FetchRegistryProfile`, `SubmitProfile`, `FormatRegistryProfiles`
- `nexus registry list|search|fetch|submit` — four new CLI subcommands
- SSRF-safe HTTP via `NewSSRFSafeTransport()` for all registry requests
- SHA256 content verification on profile fetch against registry index
- Registry index at `raw.githubusercontent.com/Sumama-Jameel/nexus-profiles/main/registry.json`
- `nexus registry submit` validates profile YAML and prints PR instructions

### Changed
- Performance: parallel goroutine matching in `ledger.QueryField()` — splits records across `runtime.NumCPU()` workers
- Version bumped to 0.15.0

## [0.14.0] - 2026-07-09

### Added
- V14: The Teleport Migration Tool (The Closer) — Windows-to-WSL2 document migration
- `internal/engine/teleport.go` — `Teleport()` walks `C:\Users\<user>` and symlinks `Documents`, `Desktop`, `Downloads`, `Pictures` into `~/`
- `nexus teleport` — single CLI command with `--dry-run` flag
- `TeleportResult` struct — per-folder outcome (linked, skipped, error)
- `TeleportSummary()` — human-readable results formatter
- `Teleported bool` in `NexusState` + `RecordTeleported()` / `IsTeleported()` — persistent state tracking
- WSL2-only guard via `info.IsWSL2` check — returns `ErrNotWSL2` on native Linux
- Symlink-only strategy: zero data copy, zero disk usage, no `os.RemoveAll`

### Security
- No `os.RemoveAll` anywhere in the teleport code — additive only
- Only operates on 4 known user folders — no arbitrary path walk
- Dry-run flag lets the user preview every action before committing
- WSL2 guard prevents accidental use on native Linux

### Changed
- Version bumped to `0.14.0`

---

## [0.13.0] - 2026-07-09

### Added
- V13: The Hardware Ledger (The Intelligence) — hardware compatibility feedback loop
- `internal/ledger/` — new bounded context with 4 source files
- `nexus ledger record|status|query|check|sync|pull` — full ledger lifecycle
- `GenerateFingerprint()` — SHA256 of (OS+Arch+CPUModel+CPUCores+GPU+RAMTotalMB), first 16 bytes → privacy-safe device ID
- `Record()` / `RecordSimple()` — full probe+install recording + manual CLI recording
- `QueryField()` — search ledger by GPU/kernel/OS/arch/cpu with `QueryReport`
- `CheckHardware()` — compare current hardware against ledger, `CheckReport`
- `Sync()` / `Pull()` — community ledger sync (stubbed, opt-in)
- `HardwareReport` / `HardwareLedger` types in `NexusState` — persistent via `StateTracker`
- `RecordLedgerEntry`, `GetLedger`, `SetCommunitySyncEnabled`, `RecordLedgerSync` — 4 new `StateTracker` methods
- Step 6 in `nexus init` — automatically records hardware report after installation
- Bounded ring: 100 records max (oldest dropped)
- Privacy-first: no hostname, IP, serial in fingerprint
- ADR 012: The Hardware Ledger

### Security
- DeviceFingerprint excludes all PII (hostname, IP, serial, disk serial)
- Community sync opt-in only (`CommunitySyncEnabled` defaults to `false`)
- SSRF-safe transport via engine's `netguard.go` for future sync HTTP calls

### Changed
- Version bumped to `0.13.0`
- Step counter in `nexus init` changed from `[5/5]` to `[5/6]` + new Step 6

---

## [0.12.0] - 2026-07-09

### Added
- V12: Container Sandbox (Distrobox) — atomic container management
- `internal/container/` — new package with 6 source files + tests
- `nexus container list|info|create|enter|apps|remove` — full container lifecycle
- `Create()` with **auto-cleanup on state write failure** — if state persistence fails after `distrobox-create` succeeds, the engine auto-removes the container via `distrobox-rm --force` and returns a wrapped error. Keeps state consistent.
- Service allowlist (Option C): hardcoded safe list + `--allow-unlisted-services` escape hatch with `MODE_APPLY_UNLISTED_SERVICE` audit
- Image ref validation: rejects `..`, `--` prefix, and invalid OCI characters
- `validate_container_name` (frontend defense-in-depth, mirrors `validate_mode_name`)
- `validate_image_ref` (frontend defense-in-depth)
- `ContainerState` + `RecordContainerCreate`/`Enter`/`Remove`/`GetContainers`/`IsContainerManaged` — persistent state tracking
- Tauri IPC: `container_list`, `container_create`, `container_remove`, `container_apps`
- Linux-only (Distrobox is Linux-native; Windows via WSL2 is future work)
- `distrobox` already in `AllowedCommands` — no new binary whitelist entries
- Test coverage: 7 table tests (internal/container)
- ADR 011: Container Sandbox

### Security
- Container names validated: `^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`
- Image refs validated: no `..`, no `--` prefix, only OCI-safe characters
- Tauri IPC validates both name and image before reaching the sidecar
- `Remove` gated on managed-state check (`--force` to override)
- Auto-cleanup prevents orphan containers when state writes fail

### Changed
- Version bumped to `0.12.0`
- Dashboard: new Containers tab (5th tab)
- Tauri Rust: `container` added to subcommand whitelist

---

## [0.11.0] - 2026-07-09

### Added
- V11: Mode Switcher (The Face) — atomic mode switching
- `internal/mode/` — new package with 6 CLI commands
- `nexus mode list|current|show|apply|rollback|define` — full mode lifecycle
- 3 built-in modes embedded via `//go:embed`: `dev`, `gamer`, `work`
- `mode.ExecFn` type alias — same type as `installer.ExecFunc` (no cross-package type errors)
- `mode.Apply` — atomic switch pipeline: validate → profile → services → os_tweaks → state → audit
- `mode.Rollback` — re-apply previous mode (no-op when no history exists)
- `mode.Define` — interactive wizard that writes `~/.nexus/modes/<name>.yaml`
- Service allowlist (Option C): hardcoded safe list + `--allow-unlisted-services` escape hatch with `MODE_APPLY_UNLISTED_SERVICE` audit
- OS tweaks: Linux `cpupower`, Windows `powercfg /setactive` with GUID resolution
- `ModeState` + `RecordModeApply`/`GetActiveMode`/`GetModeState` — persistent state tracking
- `sc`, `powercfg`, `cpupower` added to `AllowedCommands`
- CLI `--dry-run`, `--yes`, `--allow-unlisted-services` flags for `nexus mode apply`
- `Runtime` import added to `cmd/nexus/main.go`
- Test coverage: 20+ table tests (internal/mode)
- ADR 010: Mode Switcher Architecture

### Security
- Service allowlist prevents arbitrary `systemctl` / `sc` calls from mode YAML
- `--allow-unlisted-services` is audit-logged with `MODE_APPLY_UNLISTED_SERVICE`
- `--dry-run` prints the plan without any side effects
- `mode.Rollback` refuses when no previous mode is recorded (defense-in-depth)
- Built-in modes are immutable (embedded); user overrides are validated on every load

### Changed
- Version bumped to `0.11.0`
- `dashboard/src/index.html` — Profile tab replaced with Modes tab (active badge + dropdown)
- `dashboard/src-tauri/src/main.rs` — `mode` added to subcommand whitelist
- `Makefile` — `VERSION ?= 0.11.0`; new `sidecar-v11` target

---

## [0.10.0] - 2026-07-07

### Added
- V10: Tauri HUD (The Dashboard) — desktop GUI for Nexus
- `dashboard/` — Tauri v2 + React-style vanilla JS project
- `dashboard/src-tauri/` — Rust backend with Tauri commands: `read_state`, `exec_nexus_command`, `exec_nexus_with_path`
- `dashboard/src/index.html` — vanilla HTML/CSS/JS frontend (no build step required)
- Dark mode theme with CSS variables (`--bg-primary`, `--accent`, etc.)
- Four tabs: **Sync** (V8), **Vault** (V9), **System** (V1), **Profile** (V3/V7)
- Sidecar pattern: Go engine bundled as `nexus-x86_64-unknown-linux-gnu` and invoked via `tauri-plugin-shell`
- Path validation in Rust: absolute path required, must canonicalize under HOME
- Subcommand whitelist: `dotfiles`, `probe`, `version`, `config`, `profile`, `list`
- Relative-time formatting ("3m ago", "2h ago") for timestamps

### Security
- All subprocess execution flows through `tauri-plugin-shell` (no raw `Command::new` with user input)
- Path traversal blocked: `..` rejected; canonicalized paths must live under HOME
- Subcommand whitelist prevents arbitrary command execution from the frontend
- State file read restricted to `~/.nexus/state.json` with canonical-path verification

### Notes
- V10 is frontend-only — the Go engine (V1–V9) is unchanged
- The Tauri binary bundles the Go engine as a sidecar; no separate Go install required
- Frontend is vanilla JS (no React build tooling) to keep Node.js version requirements minimal
- `cargo tauri build` produces `.deb` and `.AppImage` for Linux

---

## [0.9.0] - 2026-07-07

### Added
- V9: Secrets Vault (The Shield) — `internal/dotfiles/vault.go` with 6 CLI commands
- `nexus dotfiles vault init` — generate age key pair via `age-keygen`, store private key at `~/.nexus/vault/private.key` (0600), public key at `~/.nexus/vault/public.key` (0644), optionally mirror to OS keyring
- `nexus dotfiles vault add <file>` — encrypt a sensitive file with age, place the `.age` ciphertext in the chezmoi source dir (safe to commit to any repo)
- `nexus dotfiles vault list` — show all vault-encrypted files with original path, encrypted path, size
- `nexus dotfiles vault status` — report vault initialization, key existence, permissions (0600 check), keyring accessibility, file count
- `nexus dotfiles vault unlock --key-file <path>` — install a private key on this machine (tries keyring first, falls back to file); verifies with encrypt→decrypt roundtrip before saving
- `nexus dotfiles vault remove <file>` — delete an encrypted file from the vault (removes state tracking, leaves original untouched)
- `VaultState` struct in `internal/engine/state.go` — tracks initialization, public key, key path, keyring ID, created_at, encrypted file map
- 4 new `StateTracker` methods: `RecordVaultInit`, `RecordVaultAdd`, `RecordVaultRemove`, `GetVaultState`
- OS keyring integration via `github.com/zalando/go-keyring` — private key optionally mirrored to Secret Service / Keychain / Credential Manager
- `.age` file skip in V8's secret scanner — ciphertext files are never flagged (they're safe to commit)
- V7 `add` hint: when a sensitive file is rejected, the error now suggests `vault add` for encrypted sync
- ADR 009: Secrets Vault Strategy
- Test coverage: 10+ new table tests covering vault init, add, list, status, unlock, remove

### Security
- Age (X25519 + ChaCha20-Poly1305) for all encryption — no legacy algorithms, no complex key management
- Private key at 0600 permissions in 0700 directory — defense-in-depth against other users reading our key
- Roundtrip verify on `vault unlock` — encrypts known plaintext, decrypts with provided key; if decryption fails or produces wrong output, the key is rejected
- `VaultState` tracks every encrypted file — vault operations never touch the original

### Changed
- `internal/dotfiles/secretscan.go` — added `ScanFile(path)` that skips `.age` files
- `internal/dotfiles/sync.go` — updated `scanStagedFiles` to use `ScanFile` (supports `.age` skip)
- `cmd/nexus/main.go` — `vault` subcommand registered under `dotfiles`; 6 new run functions

---

## [0.8.0] - 2026-07-07

### Added
- V8: Git-Sync Engine (The Cloud) — `internal/dotfiles/sync.go` with 3 new CLI commands
- `nexus dotfiles push [--message "..."] [--force] [--token <pat>] [--dry-run]` — stage local changes, scan for secrets, commit, push to remote
- `nexus dotfiles pull [--rebase] [--token <pat>] [--dry-run]` — fetch from remote, apply (uses `--ff-only` by default for safety)
- `nexus dotfiles sync [--message "..."] [--rebase] [--force] [--token <pat>] [--dry-run]` — convenience: pull + apply + push in one shot
- `internal/dotfiles/secretscan.go` — pre-push regex scanner (AWS keys, GitHub PATs classic + fine-grained, PEM/OpenSSH private keys, Slack tokens, generic API key assignments); refuses push by default, `--force` to override (audit-logged)
- `internal/dotfiles/sync.go` — `Push`, `Pull`, `Sync` with `SyncDeps{ExecFn, State, Audit, Token, SkipSecretScan}` and `SyncReport{Operation, Source, CommitSHA, FilesScanned, SecretsFound, Pushed, Pulled, Applied, StartedAt, CompletedAt}`
- SSH scheme support in `engine.ValidateURL` (peer to HTTPS) — accepts `ssh://git@host/path` and normalizes SCP-style `git@host:path` to canonical form
- `internal/dotfiles/source.go` — `NormalizeSourceURL` converts SCP-style git URLs to `ssh://` URLs for validation
- `DotfilesState` extended with `LastPushedAt`, `LastPulledAt`, `LastCommitSHA` (all optional via `omitempty`)
- `DotfilesSyncStatus` computed struct in `engine` for `nexus dotfiles status` and JSON output
- 3 new `StateTracker` methods: `RecordDotfilesPush(sha)`, `RecordDotfilesPull(sha)`, `GetDotfilesSyncStatus()`
- PAT handling via `--token` flag and `NEXUS_DOTFILES_TOKEN` env var; injected into remote URL for single exec call only (never persisted, never logged)
- `--ff-only` conflict safety by default for pull (refuses non-fast-forward, forces user to inspect divergence)
- `--rebase` flag for the multi-machine workflow where replaying local commits is safe
- Audit codes: `DOTFILES_PUSH`, `DOTFILES_PULL`, `DOTFILES_SYNC` (via existing `AuditEntry.Action` field)
- ADR 008: Git-Sync Strategy
- Test coverage: 30+ new table tests (secretscan, sync) + extended source tests for SSH scheme support

### Security
- Zero-Trust boundary preserved: V8 commands shell out to `chezmoi git` via existing `SanitizeAndExecute` gate (`git` was already in `AllowedCommands`, no allowlist changes)
- URL re-validated on every push/pull (defense-in-depth: in case `AllowedSourceHosts` is tightened between binds)
- PAT never logged, never written to `state.json`, never written to `.git/config` — injected into URL for single exec call only
- Secret scanner uses high-confidence patterns (fixed prefix + length) to minimize false positives
- `--force` overrides are recorded in audit log for forensic traceability
- `--ff-only` default prevents silent overwrites when local and remote diverge

### Changed
- `internal/engine/netguard.go` — `ValidateURL` now accepts `ssh://` scheme with userinfo `git` (the only user git's SSH server accepts); existing HTTPS checks unchanged
- `internal/dotfiles/source.go` — `BindSource` now calls `NormalizeSourceURL` before validation, enabling SCP-style URLs

### Notes
- V8 completes Phase 3 Slice 2 of 3 (Identity & Security). V9 (Secrets Vault / age encryption) is the final slice in this phase.
- V8 inherits all V7 security guarantees (Zero-Trust command execution, SSRF guard, audit logging, state atomicity).
- See `docs/adr/008-git-sync-strategy.md` for the full design rationale (why shell-out, why both SSH+HTTPS, why `--ff-only` default, why secret scan refuses by default).

---

## [0.7.0] - 2026-07-07

### Added
- V7: Chezmoi Integration (The Memory) — `internal/dotfiles/` package with 9 CLI commands
- `nexus dotfiles detect` — probe for chezmoi installation and report version
- `nexus dotfiles install` — install chezmoi via the Orchestrator (apt/pacman/dnf/apk)
- `nexus dotfiles init <repo-url>` — bind a Git dotfile repository (HTTPS + host whitelist + DNS SSRF check)
- `nexus dotfiles remove` — unbind the dotfile source from Nexus state
- `nexus dotfiles apply` — apply managed dotfiles from the bound source (with `--dry-run`)
- `nexus dotfiles status` — show current chezmoi state
- `nexus dotfiles diff` — show pending changes between source and live system
- `nexus dotfiles add <path>` — track an existing file with chezmoi (strict path validation, `--force` for sensitive paths)
- `nexus dotfiles verify` — verify the live system matches the bound source
- Profile schema extension: `dotfiles:` section with `source`, `apply_on_init`, `managed_paths`, `sensitive_paths`
- `DotfilesSpec` type in `pkg/manifest`
- `DotfilesState` in `~/.nexus/state.json` (Installed, Version, InstalledAt, Source, InitializedAt, LastAppliedAt, ManagedFiles)
- 7 StateTracker methods for dotfile operations (`RecordDotfilesInstall`, `RecordDotfilesInit`, `RecordDotfilesApply`, `RecordDotfilesAdd`, `RecordDotfilesRemove`, `GetDotfilesState`, `IsDotfilesInstalled`)
- `internal/engine/netguard.go` — shared SSRF primitives (`ValidateURL`, `ValidateURLAgainstHosts`, `NewSSRFSafeTransport`, `IsPrivateIP`)
- `internal/dotfiles/profile.go` — `ApplyFromProfile` bridges manifest declarations to dotfile operations
- `nexus init` now invokes `ApplyFromProfile` when the active profile has a `dotfiles:` section (Step 4b)
- `nexus profile apply` now applies dotfiles from the profile's `dotfiles:` section
- Allowed dotfile source hosts: github.com, gitlab.com, bitbucket.org, codeberg.org
- ADR 007: Chezmoi Shell-Out Strategy
- Test coverage: 30+ new table tests in `internal/dotfiles/`

### Security
- Zero-Trust boundary preserved: dotfiles package NEVER calls `exec.Command` directly; every subprocess goes through `engine.SanitizeAndExecute`
- SSRF protection on dotfile source URLs: HTTPS only, no userinfo/query/fragment, host whitelist, DNS-level private IP rejection
- Path validation for `add`: must be absolute, normalized, under `$HOME`, no traversal, no shell metacharacters
- Sensitive path protection: `.ssh/id_*`, `.gnupg/`, `.aws/credentials`, `.netrc`, etc. require `--force` to manage
- Refactored `internal/wsl/downloader.go` to share the new `netguard.go` primitives (single source of truth for SSRF)

### Changed
- All Go imports converted from `github.com/...` to local `nexus-engine/...` module path (private project, not OSS)
- `internal/wsl/downloader.go`: removed duplicate `validateDownloadURL`, `isPrivateIP`, `mustParseCIDR`, inline transport — all replaced by `engine.ValidateURL`, `engine.NewSSRFSafeTransport`, `engine.IsPrivateIP`

### Notes
- V7 begins Phase 3 (Identity & Security). The strategic milestone for V7-V9 is "User Trust Established".
- V7 leaves the data fields and validation shape in place for V8 (Git-Sync) and V9 (Secrets Vault / age) without further refactoring.
- See `docs/adr/007-chezmoi-shell-out-strategy.md` for the full design rationale.

---

## [0.6.0] - 2026-03-05

### Added
- Comprehensive test suite for all packages (engine, installer, bridge, wsl, manifest)
- GoDoc documentation on all exported symbols across all packages
- GitHub Actions CI/CD workflows (test, lint, build)
- GoReleaser configuration for automated multi-platform releases
- MIT License
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
- `Orchestrator` — 7-step flow (PreFlight → RefreshIndex → Order → Execute → Verify → Record → Audit → Report)
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
