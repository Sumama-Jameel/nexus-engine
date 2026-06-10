# Security Policy

## Supported Versions

| Version | Supported |
|---|---|
| v0.6.x | :white_check_mark: |
| < v0.6.0 | :x: |

Only the latest release line receives security updates. We recommend always running the most recent version.

---

## Reporting a Vulnerability

**Do not report security vulnerabilities through public GitHub issues, discussions, or pull requests.**

### How to Report

Send a detailed report to **[sumamajamil2005@gmail.com](mailto:sumamajamil2005@gmail.com)**.

Please include:

1. **Description** — A clear description of the vulnerability
2. **Affected component** — Which package or function is affected (e.g., `internal/engine/execute.go`, `internal/wsl/downloader.go`)
3. **Reproduction steps** — Exact commands or code to trigger the vulnerability
4. **Impact** — What an attacker could achieve (e.g., arbitrary command execution, file read, SSRF)
5. **Environment** — OS, architecture, Nexus version
6. **Suggested fix** — If you have one (optional but appreciated)

### Response Timeline

| Milestone | Target |
|---|---|
| **Acknowledgment** | Within 48 hours of receipt |
| **Initial assessment** | Within 5 business days |
| **Fix or mitigation** | Depends on severity; critical issues prioritized |
| **Disclosure** | 90 days from acknowledgment, or after fix is released (whichever comes first) |

We will keep you informed of progress throughout the process.

### What to Expect

- You will receive an acknowledgment within 48 hours confirming we received your report.
- A maintainer will assess the vulnerability and determine its severity.
- We will work on a fix and coordinate disclosure timing with you.
- You will be credited in the advisory (unless you request anonymity).

---

## Security Model

Nexus Protocol operates on a **Zero-Trust execution model**. The following mechanisms form the security architecture:

### Command Execution Gate

All shell command execution passes through `SanitizeAndExecute` in `internal/engine/execute.go`:

- **Allowlisted commands** — Only pre-approved commands can be executed. Unknown commands are rejected.
- **No shell metacharacters** — Arguments are passed directly to `exec.Command` without shell interpretation, preventing injection via `;`, `|`, `$()`, backticks, etc.
- **Context timeouts** — Every command runs with a 60-second context deadline to prevent hanging.
- **Structured error classification** — Errors are classified by type (network, permission, not-found, unknown) for safe error handling.

### Input Validation

- **Profile names** — Regex pattern `^[a-z0-9][a-z0-9-]*$` enforced at schema and Go levels. Prevents path traversal (e.g., `../../etc/passwd`).
- **Distro names** — Alphanumeric with hyphens, max 64 characters, no consecutive hyphens.
- **File paths** — `..` traversal prevention in all install and import paths.
- **JSON Schema** — Two-layer validation: JSON Schema (pattern constraints, `additionalProperties: false`) plus Go semantic validation (family mapping, empty checks, self-extends prevention).

### Integrity Verification

- **SHA256 hashing** — All profiles and rootfs images are hashed using `crypto/sha256` (FIPS-compliant, zero external dependencies).
- **Constant-time comparison** — Integrity checks use `crypto/subtle.ConstantTimeCompare` to prevent timing side-channel attacks.
- **Registry tracking** — Profile hashes are stored in `~/.nexus/profiles/registry.json` and verified on every load.

### Network Security

- **HTTPS-only** — All remote downloads require HTTPS connections. HTTP URLs are rejected.
- **SSRF protection at DNS level** — Custom HTTP transport with a `DialContext` hook that rejects private IPs (RFC1918, loopback, link-local, IPv6 ULA) before the connection is established.
- **Host whitelist** — Remote profile fetching is restricted to allowed hosts only (`raw.githubusercontent.com`, `github.com`, `gist.githubusercontent.com`). No user-supplied URLs.
- **URL validation** — No userinfo, no query parameters, no fragments in download URLs.
- **Response size limit** — 1MB maximum for profile downloads to prevent memory exhaustion.
- **Validate before persist** — Remote content is validated against the JSON Schema before being written to disk, preventing persistence of malicious profiles.

### State and Audit

- **Atomic state writes** — `~/.nexus/state.json` uses write-to-temp-then-rename for crash safety.
- **Append-only audit log** — `~/.nexus/audit.log` uses `O_APPEND` flag with JSONL format. Entries cannot be modified after writing.
- **Mutex-protected state** — Concurrent access to state is protected by `sync.Mutex`.

### WSL2 Security

- **Security-hardened wsl.conf** — Imported distros use `noexec` on automounts to prevent cross-OS malware execution.
- **Embedded setup scripts** — The post-import setup script is embedded in the Go binary, not downloaded or user-supplied. This eliminates shell metacharacter injection surfaces.
- **Nexus-managed distro checks** — Only distros imported by Nexus can be removed via `nexus wsl remove`, preventing accidental removal of user-created distributions.

---

## Scope

The following are considered **in scope** for vulnerability reports:

| Category | Examples |
|---|---|
| **Command injection** | Bypassing `SanitizeAndExecute` allowlist; executing arbitrary commands |
| **Privilege escalation** | Gaining elevated permissions through Nexus operations |
| **SSRF** | Bypassing private IP checks, host whitelist, or HTTPS enforcement |
| **Path traversal** | Reading or writing files outside intended directories |
| **Integrity bypass** | Forging SHA256 hashes or bypassing schema validation |
| **Input validation bypass** | Injecting malicious content through profile names, distro names, or YAML fields |

---

## Out of Scope

The following are **not in scope** for this vulnerability reporting process:

| Category | Reason |
|---|---|
| **Denial of Service (DoS)** | Nexus is a client-side tool; DoS requires local access |
| **Social engineering** | Phishing, impersonation, or manipulation of maintainers |
| **Third-party dependencies** | Report to the upstream project; we will update our dependencies promptly |
| **Theoretical vulnerabilities** | Issues that cannot be demonstrated with a reproducible exploit |
| **Issues in unreleased code** | Only supported versions are in scope |
| **Missing security features** | Feature requests for new security controls should go through the standard feature suggestion process |

---

## Safe Harbor

We value the security research community and believe that responsible disclosure makes our project stronger.

If you act in good faith and follow this security policy:

- We will not pursue legal action against you.
- We will work with you to understand and resolve the issue.
- We will credit you in the security advisory (unless you request anonymity).

We ask in return that you:

- Report vulnerabilities privately (not through public channels).
- Provide sufficient detail for us to reproduce and fix the issue.
- Allow us reasonable time to address the vulnerability before public disclosure.
- Do not access or modify user data, or degrade system performance beyond what is necessary to demonstrate the vulnerability.

We are committed to working with the security community to verify and address potential issues in a timely and transparent manner.

---

## Security-Related Configuration

Users can further harden their Nexus installation:

- **`--skip-verify` flag** — Skips SHA256 verification. **Dangerous**; only use in development or air-gapped environments with known-good images.
- **Remote host whitelist** — The `AllowedRemoteHosts` list in `pkg/manifest/store.go` controls which hosts can serve profiles. Modifying this list changes the SSRF attack surface.
- **Command allowlist** — The `AllowedCommands` list in `internal/engine/execute.go` controls which commands can be executed. Adding commands increases the attack surface.

Any modifications to these lists should be reviewed carefully and accompanied by a security justification.

---

*Last updated: 2026-03-05*

---

## License

Nexus Protocol Engine is licensed under the [Apache License 2.0](LICENSE). Security researchers acting in good faith under the safe harbor provisions above are not violating the license terms.
