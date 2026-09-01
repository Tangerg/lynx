# tRPC-Agent-Go: a broad-surface agent runtime

Evidence baseline: `trpc-agent-go` commit
`91bde85eb243333b2b33fe89061f2218ede00c99`.

## Framework-level judgment

tRPC-Agent-Go covers agents, runners, invocations, sessions, graphs, models,
tools, and several extension mechanisms at once. It pursues a fairly complete
one-stop Go agent development surface, and its design center is broader than
Scope's.

An earlier draft folded OpenClaw and built-in application capabilities into the
judgment, conflating framework with product. This round looks at framework
contracts only; OpenClaw, the command line, and prebuilt tools do not score.

## Reviewable evidence

- `agent/invocation.go`: `Invocation` aggregates run identity, agent, session,
  model, and run options.
- The agent and runner path: an agent executes through an invocation and an
  event stream.
- Graph: uses general state and supports nodes, edges, and checkpoint-related
  capabilities.
- The checkpoint model carries a parent checkpoint identity, so it can express
  graph history relationships.
- Callbacks, hooks, plugins, and runner extensions provide several distinct
  integration paths.
- Model, tool, knowledge, and session capabilities are spread across the main
  module and submodules.

## The eight dimensions

| Dimension | tRPC-Agent-Go's actual trade-off | Key difference from Scope |
| --- | --- | --- |
| Protocol boundary | A unified model, message, and tool surface, with broad runtime coverage | Scope's core is smaller and isolates provider and product capabilities more finely |
| Minimal contract | Agent, Invocation, and Runner cooperate | Scope's Definition and Execution are narrower with stronger recovery requirements |
| State ownership | Invocation, Session, and graph state hold it jointly | Scope's host and execution ownership is more explicit |
| Side effects | Agents, runners, or graph nodes execute directly | Scope forms one external-work boundary through Effects |
| Orchestration | Graph and agent patterns coexist | Scope Workflow represents managed child execution only |
| Recovery | Session and graph checkpoints are fairly rich | That does not make every external agent call idempotently replayable |
| Extension | Several callback, hook, and plugin surfaces | Broad capability, but a more scattered lifecycle mental model |
| Dependencies | Many one-stop capabilities, so a relatively wide minimum dependency surface | Scope can be selected leaf by leaf, at a higher maintenance cost |

## What Scope should learn

1. **Graph history and parent checkpoint expression.** Directly valuable for
   branching, backtracking, and debugging.
2. **A developer-facing integration path.** A broad framework lowers
   first-contact cost; Scope should improve composition documentation and the
   release experience without polluting the kernel.
3. **The session and runner usage pattern.** Even though Scope leaves product
   sessions to the host, the boundary collaboration can be easier to discover.

## What Scope should not copy

- Do not re-aggregate models, sessions, knowledge, tools, runners, and product
  capabilities into a wide invocation.
- Do not run several extension lifecycles side by side without being able to
  state their priority.
- Do not advertise graph checkpoints as a recovery guarantee for arbitrary
  external side effects.
- Do not move application-layer capability back into the Scope repository just
  because a peer ships more built-in applications.

## Final placement

tRPC-Agent-Go suits Go agent projects that want a wide feature surface and
one-stop composition. Scope suits making execution semantics an independent,
recoverable kernel and letting applications such as Flame assemble product
capability themselves. The difference is mainly boundary width, not feature
count.
