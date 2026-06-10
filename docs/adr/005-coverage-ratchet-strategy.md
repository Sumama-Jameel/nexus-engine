# ADR 005: Coverage Ratchet Strategy

## Status
Accepted

## Context
Test coverage at the start of the improvement initiative was 46%. The target was 78-82%. Without a mechanism to prevent regression:

1. Coverage could silently decrease as new code is added without tests
2. There was no CI gate to catch coverage regressions before merge
3. Teams had no visibility into coverage trends

## Decision
Implement a **coverage ratchet** strategy:

1. **Absolute floor**: Total coverage must never fall below 80% (enforced in CI)
2. **Per-package minimums**: Each package has its own floor (e.g., installer at 90%)
3. **Ratchet file**: `.coverage-threshold` stores the current best coverage
4. **Auto-update**: When coverage improves, the threshold file is updated
5. **CI enforcement**: The `coverage-ratchet.sh` script runs in CI to prevent regression

The ratchet only goes UP — coverage can never decrease below the recorded threshold.

```
Sprint 1: 46% → 51% (ratchet: 51%)
Sprint 2: 51% → 57% (ratchet: 57%)
Sprint 3: 57% → 65% (ratchet: 65%)
Sprint 4: 65% → 73% (ratchet: 73%)
Sprint 5: 73% → 80% (ratchet: 80%)  ← floor established
Sprint 6: 80% → 82.5% (ratchet: 82.5%) ← current
```

## Consequences
- **Positive**: Coverage can only increase — regression is structurally prevented
- **Positive**: Per-package minimums catch localized regressions even when total is fine
- **Positive**: CI gate provides immediate feedback on PRs
- **Positive**: `.coverage-threshold` file makes coverage visible in git history
- **Negative**: Developers must run coverage locally before pushing
- **Negative**: 80% floor may be too high for some packages initially

## References
- Coverage ratchet pattern: https://medium.com/patto-continuous-delivery/coverage-ratchet-e36c2c6b0704
- Similar to "build cop" pattern in CI/CD
