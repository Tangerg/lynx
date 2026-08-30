# Eval Module Rules

Read the repository root `CLAUDE.md` before this file. These rules define the
additional contract for the `eval` module.

## Scope

`eval` is a subject-agnostic experiment runtime. The root package owns
datasets, evaluators, reports, suites, experiments, comparisons, aggregation,
and in-memory result projections. Domain packages own only their vocabulary:

- `judge` adapts model judgments to typed reports.
- `ranking` evaluates ordered candidates.
- `text` evaluates text quality dimensions.

RAG, Agent, and application concepts must reach this module through typed
subjects and evaluators; they must not become root evaluation primitives.

## Ownership

- `Dataset`, `Case`, `Report`, and returned experiment values own their mutable
  data. Callers never receive aliases to framework state.
- `Metric` identity includes its name, unit, direction, and parameters. Reports
  with different identities must never be aggregated together.
- `Report.Details` is a bounded tree. All construction, clone, JSON, and summary
  boundaries must preserve `MaxReportDepth`.
- `ErrorCollect` records case failures and continues. `ErrorFailFast` cancels
  unscheduled work but does not erase already settled case facts.

## Negative Invariants

- Do not invent a pass threshold, normalized score, or verdict for evaluators
  that only produce measurements or qualitative feedback.
- Do not combine different units or metric identities into one synthetic score.
- Do not claim statistical significance without an explicit statistical model
  and sufficient inputs.
- Do not own dataset persistence, artifact storage, experiment tracking services,
  dashboards, product identities, or deployment workflows. Those belong to a
  Host or application layer.
- Do not add RAG-specific evaluators to the root package. Keep domain vocabulary
  in a focused subpackage and adapt it through the generic evaluator protocol.
