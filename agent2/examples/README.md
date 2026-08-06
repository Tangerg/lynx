# Agent Framework examples

These commands are disposable consumers of the public `agent2` API. They do
not share test-only helpers or import internal protocol types, so build and test
failures expose real consumer-facing contract problems.

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

All examples use deterministic local components so they run without credentials or
network access:

```sh
GOWORK=off go run ./examples/direct_vs_managed
GOWORK=off go run ./examples/autonomous
GOWORK=off go run ./examples/composition
GOWORK=off go run ./examples/workflow
GOWORK=off go run ./examples/orchestrator_workers
```
