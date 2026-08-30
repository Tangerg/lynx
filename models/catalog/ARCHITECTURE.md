# catalog architecture

> Embedded provider reference data — model identity, pricing, capabilities,
> modalities, and token limits — independent from any invocation protocol.

The family contract is [`../ARCHITECTURE.md`](../ARCHITECTURE.md);
repository-wide rules are in [`../../CLAUDE.md`](../../CLAUDE.md). The usage
entry point is [`README.md`](README.md).

---

## 1. What this module owns

- The embedded catalog data and its validated shape.
- Lookup over that data by provider and model identity.

## 2. What it does not own

- Model invocation. It implements no Core `Model` contract and holds no SDK.
- Model routing or selection policy. The catalog supplies facts; the caller
  decides.
- Freshness guarantees beyond the embedded snapshot. Updating the data is a
  repository change, not a runtime fetch.

## 3. Dependency island

This module is an independent release unit with no provider SDK requirement. It
must never import a sibling provider, and a provider must never depend on it to
function — a model call works without the catalog present.
