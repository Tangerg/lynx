# CLAUDE.md — agent2 module

> Greenfield replacement for the existing Agent Framework. Project-wide rules
> are inherited from [`../CLAUDE.md`](../CLAUDE.md).

Before designing, implementing, reviewing, or refactoring this module, read the
following documents completely:

1. [`doc/ARCHITECTURE.md`](doc/ARCHITECTURE.md) — accepted target architecture and boundaries.
2. [`doc/DECISIONS.md`](doc/DECISIONS.md) — architecture decisions and their rationale.
3. [`doc/ENGINEERING_STANDARDS.md`](doc/ENGINEERING_STANDARDS.md) — mandatory implementation and quality standard.
4. [`doc/EXECUTION_PLAN.md`](doc/EXECUTION_PLAN.md) — authorized scope, phases, progress, and verified facts.

The documents have distinct owners. Do not copy progress into architecture,
copy architecture into the execution log, or rewrite accepted decisions without
a superseding ADR.

Until the consumer migration phase begins:

- modify only `agent2` and workspace/module metadata required to build it;
- use the existing `agent` source and tests as implementation evidence, never as
  a compatibility contract or imported dependency;
- do not modify `app/runtime`, frontend, TUI, or CLI to accommodate incomplete
  framework work;
- do not commit placeholders, compatibility shims, known debt, or partial
  semantics; and
- update the execution plan after every completed and verified batch.
