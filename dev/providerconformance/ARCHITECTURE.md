# providerconformance architecture

> Cross-provider construction and API consistency, checked where all providers
> are visible at once.

Repository-wide rules live in [`../../CLAUDE.md`](../../CLAUDE.md); the usage
entry point is [`README.md`](README.md).

---

## 1. What this module owns

- Consistency checks across the whole provider family: construction shape,
  public surface, and error classification vocabulary.

## 2. What it does not own

- Per-provider behavior. That is `core/modeltest`, run by each provider module
  against its own implementation.
- Any production behavior. Nothing here ships.

## 3. Dependency direction

This module may depend on every provider, which is precisely why it exists
separately: a provider must never import a sibling, so the comparison cannot
live inside one of them. No product module may import this one.

## 4. Read before changing

A new provider joins these checks by existing. If adding one makes a check fail,
the provider is inconsistent with the family — fix the provider before relaxing
the check.
