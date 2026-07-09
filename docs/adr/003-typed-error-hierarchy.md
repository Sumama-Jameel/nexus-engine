# ADR 003: Typed Error Hierarchy for Orchestrator Flow

## Status
Accepted

## Context
The Orchestrator has a 7-step flow (PreFlight → RefreshIndex → Order → Execute → Verify → Record → Audit → Report) with 3 inviolable rules. When errors occurred, they were generic `fmt.Errorf` strings with no structure. This meant:

1. Callers couldn't distinguish a pre-flight failure from an installation failure
2. The rollback mechanism couldn't detect *why* a failure occurred (foundation vs. tool)
3. Error messages were inconsistent and couldn't be machine-parsed

## Decision
Create a typed error hierarchy rooted at `NexusError`:

```go
type NexusError struct {
    Code    string // Machine-readable code (e.g., "PREFLIGHT_FAIL")
    Message string // Human-readable description
    Stage   string // Which Orchestrator step failed
    Cause   error  // Wrapped underlying error
}

// Constructors enforce stage-specific codes:
func NewPreflightError(msg string, cause error) *NexusError
func NewFoundationError(msg string, cause error) *NexusError
func NewVerifyError(msg string, cause error) *NexusError
func NewRollbackError(msg string, cause error) *NexusError
func NewInstallError(msg string, cause error) *NexusError
```

The `NexusError` type implements `error` with `Error()`, `Unwrap()`, and preserves the full cause chain.

## Consequences
- **Positive**: Callers can use `errors.As(err, &nexusErr)` for structured error handling
- **Positive**: Orchestrator can detect foundation failures and trigger rollback
- **Positive**: Error codes are machine-readable for API/IPC integration
- **Positive**: 100% coverage on error types and constructors
- **Negative**: Slightly more verbose than `fmt.Errorf`
- **Negative**: Must ensure all error sites use the typed constructors

## References
- Go error handling best practices: https://go.dev/blog/go1.13-errors
- Domain-driven error types: https://martinfowler.com/articles/replaceThrowWithNotification.html
