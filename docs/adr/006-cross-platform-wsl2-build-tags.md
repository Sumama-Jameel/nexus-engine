# ADR 006: Cross-Platform WSL2 via Build Tags

## Status
Accepted

## Context
WSL2 import operations only work on Windows. The codebase needs to compile on all platforms (Linux, Windows, macOS) but WSL2-specific code can only execute on Windows. Initial attempts had:

1. Runtime `if runtime.GOOS == "windows"` checks scattered everywhere
2. Concrete Windows types leaked into Linux code, causing compilation errors
3. Tests couldn't validate platform-specific behavior

## Decision
Use Go build tags (`//go:build`) to split platform-specific code:

- `wsl_windows.go` — Real WSL2 implementation (Windows build tag)
- `wsl_linux.go` — Stubs that return "not available" errors (Linux build tag)

The Linux stubs provide:
- Identical type definitions (for compilation compatibility)
- No-op methods that return `NotAvailableError()`
- `IsImportAvailable() bool` returning `false` on Linux, `true` on Windows

The runner layer uses `WSLAvailable bool` flag + `WSLListFn`/`WSLImagesFn` function injection to make WSL delegation testable on all platforms.

## Consequences
- **Positive**: Code compiles on all platforms without `#ifdef` or runtime checks
- **Positive**: Linux stubs provide clear error messages ("only available on Windows")
- **Positive**: Tests can verify delegation logic on Linux by setting `WSLAvailable = true`
- **Positive**: No dead Windows code included in Linux binary
- **Negative**: Stub types must be kept in sync with Windows types manually
- **Negative**: Some WSL functions have 0% coverage on Linux (they return errors immediately)

## References
- Go build constraints: https://pkg.go.dev/go/build#hdr-Build_Constraints
- Similar pattern: https://github.com/golang/go/wiki/WindowsCrossCompilation
