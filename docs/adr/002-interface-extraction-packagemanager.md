# ADR 002: Interface Extraction for PackageManager

## Status
Accepted

## Context
The installer package had concrete types (`AptInstaller`, `PacmanInstaller`, `DnfInstaller`, `ApkInstaller`) but no interface. The Orchestrator and Runner needed to work with *any* package manager, but:

1. The Orchestrator accepted `*AptInstaller` directly — breaking for other package managers
2. Tests couldn't mock package manager behavior — they needed a real system
3. Adding a new package manager required modifying the Orchestrator

## Decision
Extract the `PackageManager` interface from the concrete implementations:

```go
type PackageManager interface {
    RefreshIndex(ctx context.Context) error
    Install(ctx context.Context, packages []string) ([]PackageResult, error)
    Remove(ctx context.Context, packages []string) ([]PackageResult, error)
    Update(ctx context.Context, packages []string) ([]PackageResult, error)
    IsInstalled(ctx context.Context, pkg string) bool
    ListInstalled(ctx context.Context) ([]string, error)
    Search(ctx context.Context, query string) ([]string, error)
    Name() string
}
```

The `NewInstaller` factory function returns `PackageManager` (not a concrete type):
```go
func NewInstaller(family string, execFn engine.ExecFunc) (PackageManager, error)
```

## Consequences
- **Positive**: Orchestrator and Runner depend on `PackageManager` interface, not concrete types
- **Positive**: Tests inject mock implementations with controlled behavior
- **Positive**: New package managers (e.g., `ZypperInstaller`) can be added without modifying consumers
- **Positive**: Installer package coverage reached 98.3%
- **Negative**: Interface must be updated when adding methods (RefreshIndex was added in V2 gap audit)

## References
- Dependency Inversion Principle: https://en.wikipedia.org/wiki/Dependency_inversion_principle
- Factory Pattern: Go standard library pattern (e.g., `hash.New`)
