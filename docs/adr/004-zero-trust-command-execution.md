# ADR 004: Zero-Trust Command Execution via SanitizeAndExecute

## Status
Accepted

## Context
The Nexus Protocol requires that "every system call the Go engine makes to the shell must pass through a centralized SanitizeAndExecute function." Without this:

1. Any code could execute arbitrary shell commands (command injection risk)
2. There was no audit trail of what commands were run
3. Timeouts were inconsistent (some commands hung indefinitely)

## Decision
Implement a single security gate function `SanitizeAndExecute` that ALL shell command execution must pass through:

```go
func SanitizeAndExecute(ctx context.Context, command string, args ...string) (string, error)
```

This function enforces:
1. **Whitelist**: Only pre-approved commands (apt-get, pacman, dnf, apk, which, etc.) can be executed
2. **Metacharacter rejection**: Arguments containing `;`, `|`, `$`, backticks, etc. are rejected
3. **Timeout**: All commands run with a 60-second context timeout
4. **Audit**: All invocations are logged

The function is injected via `ExecFunc` type into all packages that need shell execution:
- `installer.ExecFunc` — used by all package manager implementations
- `bridge.ExecFunc` — used for environment detection on Windows
- `wsl.ExecFunc` — used for WSL2 import operations

## Consequences
- **Positive**: Zero raw `exec.Command` calls exist in production code
- **Positive**: Command injection is structurally prevented
- **Positive**: All execution is audit-logged
- **Positive**: Tests can inject mock `ExecFunc` for controlled behavior
- **Negative**: Adding new commands requires whitelist update
- **Negative**: Legitimate arguments with special characters (e.g., package names with `+`) might be rejected — use `AllowCommandWithArgs` for known-safe patterns

## References
- Zero-Trust Architecture: https://www.nist.gov/publications/zero-trust-architecture
- Command Injection Prevention: OWASP https://cheatsheetseries.owasp.org/cheatsheets/Command_Injection_Prevention_Cheat_Sheet.html
