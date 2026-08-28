# CLAUDE.md — agent module

> Scope Agent Framework library. Project-wide rules are inherited from
> [`../CLAUDE.md`](../CLAUDE.md).

Before changing production code in this module, read these current contracts
completely:

1. [`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md) — accepted architecture and boundaries.
2. [`doc/ENGINEERING_STANDARDS.md`](doc/ENGINEERING_STANDARDS.md) — mandatory implementation and quality standard.

Read the remaining documents when the change crosses their owner boundary:

- [`doc/API_BASELINE.md`](doc/API_BASELINE.md) before changing exported API,
  GoDoc, derived schema, snapshot/state/protocol, or observation wire;
- the relevant accepted or superseding entries in
  [`doc/DECISIONS.md`](doc/DECISIONS.md) before changing an architectural
  decision, and append a new ADR rather than rewriting history;
- use Git history when historical migration evidence is needed; completed plans,
  ledgers, and dated comparison snapshots do not live in the current contract set.

The documents have distinct owners. Do not copy progress into architecture,
copy architecture into the execution log, copy architecture into the capability
ledger, or rewrite accepted decisions without a superseding ADR.

Framework work follows these module rules:

- keep production changes inside `agent` and the workspace/module metadata
  required to build it unless an explicit consumer migration authorizes more;
- use repository history as implementation evidence, never as a compatibility
  contract or imported dependency;
- do not make the Framework depend on a Host application;
- do not commit placeholders, compatibility shims, known debt, or partial
  semantics.
