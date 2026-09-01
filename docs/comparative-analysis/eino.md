# Eino: typed Runnables and graph composition

Evidence baseline: `eino` commit `0e01b2a4e3050c4027bd61f2c2e2a519aa1e237c`.

## Framework-level judgment

Eino's design center is unifying models, tools, Lambdas, and graph nodes into a
composable `Runnable[I,O]`, then building complex execution through Graph and
Workflow. It is closer to a typed AI component orchestration framework than to
a Scope-style durable process kernel.

An earlier draft called "graph nodes produce side effects directly" a defect.
That framing is unfair. For request-scoped data flow, direct execution
substantially lowers implementation cost; the absence of a unified side-effect
identity becomes a structural limit only when the goal is exact replay and
cross-process recovery.

## Reviewable evidence

- `compose/runnable.go`: `Runnable[I,O]` unifies the Invoke, Stream, Collect,
  and Transform calling shapes.
- `compose`: Graph, Workflow, Lambda, and state handling form the primary
  orchestration surface.
- The checkpoint and state implementations: graph execution can save and
  restore node state.
- The callback system: components and graph runs emit unified events.
- `schema/message.go`: a shared message model, extended by AgenticMessage for
  agent scenarios.
- Model and tool implementations arrive mostly through extension modules or an
  extension repository, so the core composition layer keeps some distance from
  concrete providers.

## The eight dimensions

| Dimension | Eino's actual trade-off | Key difference from Scope |
| --- | --- | --- |
| Protocol boundary | A shared schema and component interfaces; provider extensions are relatively separate | Both value isolation; Eino centers on component types |
| Minimal contract | Runnable unifies four synchronous and streaming shapes | Scope uses Definition and Execution to express instance and recovery |
| State ownership | Graph state, node state, and context cooperate | Scope separates host data from the execution snapshot more strictly |
| Side effects | Happen directly when a node or component executes | A Scope Step only describes an Effect |
| Orchestration | Strong arbitrary-graph and typed composition | Scope Workflow has a closed vocabulary and manages only child Processes |
| Recovery | Graph checkpoints and state serialization | Scope additionally unifies effect settlement and execution-tree recovery |
| Extension | Callbacks run through components and orchestration | Scope uses middleware and listeners, and isolates OpenTelemetry |
| Dependencies | A natural boundary between core and provider extensions | Scope's leaf modules are finer-grained, at a higher governance cost |

## What Scope should learn

1. **A unified typed experience across synchronous and streaming composition.**
   Eino's Runnable makes component substitution and graph wiring feel natural.
2. **Construction-time graph validation.** The earlier type, edge, and node
   constraints fail, the lighter the runtime recovery burden.
3. **Keep ordinary data flow lightweight.** Delegating that to the separate
   `flow` module is right; keep it interoperable rather than pushing arbitrary
   DAGs back into managed Workflow.
4. **Decouple the extension ecosystem from the kernel.** Provider and component
   extensions do not need to enter the core repository's stable surface.

## What Scope should not copy

- Do not equate arbitrary graph state with recoverable Process state.
- Do not let unidentifiable external I/O inside a node bypass the Effect
  semantics that long-running tasks depend on.
- Do not widen `agent.Execution` to carry four calling shapes; ordinary calls
  belong to a lower-level interface.

## Final placement

If the question is "how do I compose AI components and graphs type-safely",
Eino is more natural than Scope's managed Workflow. If the question is "how do
I recover an execution tree that already performed external work", Scope's
Effect and Process semantics are more complete. The two solve adjacent but
different problems.
