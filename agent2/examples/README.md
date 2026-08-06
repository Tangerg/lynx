# Agent Framework examples

These commands are disposable consumers of the public `agent2` API. They do
not share test-only helpers or import internal protocol types, so build and test
failures expose real consumer-facing contract problems.

They are contract evidence, not a recommendation to turn every function into a
Process. Prefer direct `chatclient` for simple calls and ordinary Go or `flow`
for in-process control flow; use managed examples only when independent
lifecycle, recovery, budget, cancellation, or governance is required.

- `direct_vs_managed` contrasts a direct `chatclient` call with an Engine-owned
  Interaction Process. Direct calls remain the smallest embedding level;
  managed calls add lifecycle, signals, Effects, snapshots, limits, and events.
- `autonomous` runs a model-directed `model -> Tool -> model` loop. The model
  selects the Tool and stop point while the Definition supplies a hard local
  model-call limit.
- `composition` runs one local Definition directly through an embedded Engine,
  then composes the same Definition with an Interaction as two heterogeneous
  child Processes. Both paths use the same public execution narrow waist.
- `workflow` composes a managed Call and a two-branch Fork. Its final tree has
  one root and three exact child Processes, showing that ordered orchestration
  does not create a second runtime or hide branch lifecycles.
- `orchestrator_workers` lets one Interaction decompose an objective, uses a
  bounded Workflow Map to run three exact worker child Processes in stable
  order, then lets another Interaction synthesize their typed results. Its
  tests also prove that an Interaction can select exact Planning workers as
  Delegates when those tasks have machine-verifiable goals. No Supervisor
  Strategy, worker registry, or shared Blackboard is involved.
- `evaluator_optimizer` uses a bounded Workflow Loop whose exact body Process
  calls exact optimizer and evaluator child Processes. Consumer-owned typed
  state carries ordered attempts, actionable feedback, stable best-so-far, and
  acceptance separately from exhaustion; no evaluator-specific Framework type
  or hidden callback loop is added.
- `workflow_patterns` composes two exact Calls, one selected Switch case, two
  parallel section workers, and four parallel voters in one tree. It proves
  prompt chaining, routing, sectioning, declaration-order aggregation, and a
  stable consumer-owned tie break without adding pattern-specific Framework
  types.
- `embedded_vs_platform` runs the same exact root/worker Deployments once with
  a caller-owned resolver and once with Platform discovery plus exact
  resolution. Output, terminal status, Usage, Process tree, admission facts,
  and stable Process/Step/Effect observation semantics must match; Platform
  does not wrap or replace Engine.

All examples use deterministic local components so they run without credentials or
network access:

```sh
GOWORK=off go run ./examples/direct_vs_managed
GOWORK=off go run ./examples/autonomous
GOWORK=off go run ./examples/composition
GOWORK=off go run ./examples/workflow
GOWORK=off go run ./examples/orchestrator_workers
GOWORK=off go run ./examples/evaluator_optimizer
GOWORK=off go run ./examples/workflow_patterns
GOWORK=off go run ./examples/embedded_vs_platform
```
