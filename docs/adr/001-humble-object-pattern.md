# ADR 001: Humble Object Pattern for CLI Wiring

## Status
Accepted

## Context
The `cmd/nexus/main.go` file contained 1800+ lines of mixed business logic and CLI wiring. Cobra command handlers (`runInstall`, `runRemove`, etc.) directly constructed dependency objects, called business logic, and formatted output — all in one function. This made the code untestable because:

1. Every `run*` function required a live system (real package manager, real filesystem)
2. Coverage of `main.go` was 0.3% because the functions couldn't be called from tests
3. Business logic changes required modifying CLI code, violating SRP

## Decision
Apply the **Humble Object Pattern** by splitting the codebase into:

1. **`cmd/nexus/runner/` package** — Contains all testable business logic in the `Dependencies` struct with injectable interfaces
2. **`cmd/nexus/main.go`** — Thin shell that only does CLI wiring (Cobra commands, flag parsing, output formatting)

The `runner.Dependencies` struct receives all external dependencies via constructor injection:
- `PM installer.PackageManager` (interface, not concrete type)
- `State *engine.StateTracker` (real, but uses temp dirs in tests)
- `ProfileStore ProfileStoreOps` (interface, not concrete `*manifest.ProfileStore`)
- `ExecFn installer.ExecFunc` (function injection for Zero-Trust execution)
- `WSLListFn`, `WSLImagesFn` (function injection for WSL platform differences)

## Consequences
- **Positive**: `runner` package coverage is 80.4%+ (was 0%)
- **Positive**: Business logic can be tested with mock dependencies
- **Positive**: CLI layer is thin and can be tested with Cobra test infrastructure
- **Negative**: Two packages instead of one — slightly more indirection
- **Negative**: Global variables (`outputJSON`, `dryRun`, etc.) still exist for Cobra flag binding, requiring `resetFlags()` in tests

## References
- Humble Object Pattern: https://martinfowler.com/books/meszaros.html (xUnit Test Patterns)
- Dependency Injection via constructor: https://en.wikipedia.org/wiki/Dependency_injection
