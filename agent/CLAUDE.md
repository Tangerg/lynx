# CLAUDE.md — agent module

> Scope Agent Framework library. Project-wide rules are inherited from
> [`../CLAUDE.md`](../CLAUDE.md).

Before designing, implementing, reviewing, or refactoring this module, read the
following documents completely:

1. [`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md) — accepted target architecture and boundaries.
2. [`doc/DECISIONS.md`](doc/DECISIONS.md) — architecture decisions and their rationale.
3. [`doc/ENGINEERING_STANDARDS.md`](doc/ENGINEERING_STANDARDS.md) — mandatory implementation and quality standard.
4. [`doc/EXECUTION_PLAN.md`](doc/EXECUTION_PLAN.md) — authorized scope, phases, progress, and verified facts.
5. [`doc/CAPABILITY_LEDGER.md`](doc/CAPABILITY_LEDGER.md) — old-capability ownership, verdicts, and acceptance coverage.
6. [`doc/API_BASELINE.md`](doc/API_BASELINE.md) — accepted exported API and snapshot/tree wire baseline.

The documents have distinct owners. Do not copy progress into architecture,
copy architecture into the execution log, copy architecture into the capability
ledger, or rewrite accepted decisions without a superseding ADR.

[`doc/PEER_COMPARISON.md`](doc/PEER_COMPARISON.md) records dated external-framework
evidence. It is reference material, not required reading and not a contract; a gap
it identifies becomes actionable only through a new ADR.

Framework work follows these module rules:

- keep production changes inside `agent` and the workspace/module metadata
  required to build it unless an explicit consumer migration authorizes more;
- use repository history as implementation evidence, never as a compatibility
  contract or imported dependency;
- do not make the Framework depend on `app/runtime`, frontend, TUI, CLI, or
  another Host application;
- do not commit placeholders, compatibility shims, known debt, or partial
  semantics; and
- update the execution plan after every completed and verified batch.
