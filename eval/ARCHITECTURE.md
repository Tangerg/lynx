# eval architecture

> A subject-agnostic experiment runtime. The kernel judges nothing in
> particular, so a new domain arrives as a typed subject and an evaluator rather
> than as a new root primitive.

Repository-wide rules live in [`../CLAUDE.md`](../CLAUDE.md). The usage entry
point is [`README.md`](README.md).

---

## 1. Scope

The root package owns datasets, evaluators, reports, suites, experiments,
comparisons, aggregation, and in-memory result projections. Domain packages own
only their vocabulary:

- `judge` adapts model judgments to typed reports.
- `ranking` evaluates ordered candidates.
- `text` evaluates text quality dimensions.

RAG, Agent, and application concepts reach this module through typed subjects
and evaluators. They must not become root evaluation primitives.

## 2. Ownership

- `Dataset`, `Case`, `Report`, and the values an experiment returns own their
  mutable data. A caller never receives an alias to framework state.
- `Metric` identity includes its name, unit, direction, and parameters. Reports
  with different identities must never be aggregated together.
- `Report.Details` is a bounded tree. Every construction, clone, JSON, and
  summary boundary preserves `MaxReportDepth`.
- `ErrorCollect` records a case failure and continues. `ErrorFailFast` cancels
  unscheduled work but does not erase case facts that already settled.
- Concurrency limits resolve once at the construction boundary. A zero or
  negative value is rejected there rather than deadlocking a consumer later.

## 3. Negative invariants

- Never invent a pass threshold, normalized score, or verdict for an evaluator
  that only produces measurements or qualitative feedback.
- Never combine different units or metric identities into one synthetic score.
- Never claim statistical significance without an explicit statistical model and
  sufficient inputs.
- Never own dataset persistence, artifact storage, experiment-tracking services,
  dashboards, product identities, or deployment workflows. Those belong to a
  Host or an application layer.
- Never add a RAG-specific evaluator to the root package. Domain vocabulary stays
  in a focused subpackage and reaches the kernel through the generic evaluator
  protocol.

## 4. Read before changing

- Changing `Evaluator`, `Metric`, or `Report` is a consumer contract change that
  reaches every domain package and `otel/eval`.
- Adding a domain: implement `Evaluator` in its own package. Do not reach for a
  text or ranking concept the domain does not have.
- Observability is external: `otel/eval` wraps a generic `Evaluator[T]`. This
  module never imports OpenTelemetry.
