# repoarch architecture

> Repository-wide module, package-layer, and provider-boundary invariants,
> written as tests.

Repository-wide rules live in [`../../CLAUDE.md`](../../CLAUDE.md); the usage
entry point is [`README.md`](README.md).

---

## 1. What this module owns

- The executable form of the repository's structural rules.
- Discovery of what it checks, so a new package or provider joins a gate by
  existing rather than by being remembered.

## 2. What it does not own

- Per-module invariants. Those belong to that module's own `internal/arch`
  tests, next to the code they constrain.
- Coverage budgets. Those live in `scripts/check-*-coverage.sh`.
- Any production behavior. Nothing here ships.

## 3. Dependency direction

This module may read any module's source. No product module may import it. It is
development tooling and stays outside the published dependency graph.

## 4. Read before changing

Loosening a gate is an architecture decision, not a test fix. If a gate fires,
the first question is whether the code is wrong — the gate exists because the
rule was decided deliberately.
