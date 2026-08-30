# eval/trajectory architecture

> A leaf integration that projects Agent execution into the generic eval
> runtime without teaching either kernel about the other.

Repository-wide rules live in [`../../AGENTS.md`](../../AGENTS.md). The usage
entry point is [`README.md`](README.md).

## 1. Boundary

This module depends on both `agent` and `eval` because it owns their integration.
The eval kernel remains subject-agnostic and Agent remains independent of
evaluation policy. `Recorder` implements the existing Agent `EventListener` and
Interaction `ExecutionObserver`; neither source module imports this module.

## 2. Facts and comparison

- `Trajectory` owns a complete terminal root-tree record and validates the root
  finished fact against `Result`.
- `Recorder.Take` is consuming. A long-lived Engine therefore does not acquire
  an unbounded evaluation log.
- Tool assertions compare the exact ordered name and semantic JSON arguments;
  expected outcomes are optional and typed.
- `BehaviorDigest` includes semantic transitions, model-call attribution, Tool
  behavior, and terminal output. It excludes provider response bodies because
  only their committed consequences are runtime behavior; it also excludes
  wall-clock timestamps, attempt duration, provider identity, and token
  accounting because those are not deterministic replay semantics.
- Duration, Framework `Usage`, and model tokens remain explicit regression
  measurements rather than being hidden inside the behavior digest.

## 3. Negative invariants

- Never parse `ProcessSnapshot` or `TreeSnapshot` JSON. Snapshot wire is an
  Agent-owned recovery contract, not an evaluation extension point.
- Never add execution control, replay scheduling, dataset persistence,
  experiment tracking, dashboards, or product identity.
- Never make timing or provider accounting part of deterministic equality.
- Never make eval root depend on Agent or Agent depend on eval.
