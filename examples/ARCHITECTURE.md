# examples architecture

> Runnable demonstrations of Scope. Not a library, and never a dependency.

Repository-wide rules live in [`../AGENTS.md`](../AGENTS.md); the usage entry
point is [`README.md`](README.md).

---

## 1. Position

- Each subdirectory is a `main` package showing how several modules compose.
- The root package contains only its `doc.go` overview and exports no API.

## 2. Negative invariants

- Never let a Scope module import an example. The dependency direction is
  one-way, and breaking it turns a demonstration into an API nobody agreed to
  support.
- Never put a capability a real caller would want here. That belongs in `tools`
  or the owning domain module, where it gets a contract and tests.
- Never let an example require credentials to compile or to run its offline
  path. A key-gated program states what is missing and exits.
