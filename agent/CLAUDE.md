# CLAUDE.md — agent module

> Scope Agent Framework library. Project-wide rules are inherited from
> [`../CLAUDE.md`](../CLAUDE.md).

Before changing production code in this module, read these current contracts
completely:

1. [`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md) — accepted architecture and boundaries.
2. [`doc/ENGINEERING_STANDARDS.md`](doc/ENGINEERING_STANDARDS.md) — mandatory implementation and quality standard.

Use Git history when historical migration evidence is needed. Completed plans,
decision ledgers, release baselines, and dated comparison snapshots do not live
in the current contract set.

The documents have distinct owners. Keep architecture facts in architecture and
implementation rules in engineering standards; do not add progress logs or
historical ledgers to either document.

Framework work follows these module rules:

- keep production changes inside `agent` and the workspace/module metadata
  required to build it unless an explicit consumer migration authorizes more;
- use repository history as implementation evidence, never as a compatibility
  contract or imported dependency;
- do not make the Framework depend on a Host application;
- do not commit placeholders, compatibility shims, known debt, or partial
  semantics.
