# Google ADK Go: session-driven agents and a workflow graph

Evidence baseline: `adk-go` commit
`0da17d5183cc7affd4bdb7b4075f9e264bb598be`.

## Framework-level judgment

ADK Go can no longer be summarized as an agent tree plus Sequential, Parallel,
and Loop. It also carries an independent `workflow` graph runtime: nodes and
edges, static and dynamic scheduling, concurrency control, JSON Schema
validation, human-in-the-loop, durable run state, and resume are all in the
source and its tests.

Its framework center is therefore two coexisting layers:

- Agent, InvocationContext, Session, and the event stream form a session-driven
  agent runtime.
- The workflow scheduler organizes agents, functions, tools, dynamic nodes, and
  sub-workflows into a pausable, resumable graph.

The CLI, web interface, and server shell remain application or developer
tooling and do not enter the framework judgment.

## Reviewable evidence

- `agent/agent.go`: agents are framework-constructed and emit session events
  through `iter.Seq2`.
- `internal/context/invocation_context.go`: InvocationContext centrally carries
  this call's agent, session, and service context.
- `workflow/workflow.go`: `Node`, `Edge`, Route, Workflow, and the scheduler
  entry point; node inputs and outputs can declare a JSON Schema.
- `workflow/scheduler.go`: a single consumer owns state commit while node tasks
  execute concurrently, with a maximum concurrency and a pending queue.
- `workflow/state.go`: RunState and NodeState hold status, input, output,
  branch, interrupt, attempt, and resume inputs.
- `workflow/persistence.go` and `resume.go`: rebuild paused state from session
  history, validate responses, and resume waiting nodes.
- `workflow/request_input.go`: a HITL request uses a stable interrupt ID and a
  response schema.
- `agent/workflowagents/`: Sequential, Parallel, and Loop agents still coexist
  with the new workflow surface.
- `plugin/plugin.go`: run, message, and event lifecycle plugins.

## The exact boundary of its recovery

ADK Workflow's recovery is a real implementation and should not be downgraded
to ordinary chat history:

- RunState can be written into `session.State`.
- A running node can be rescheduled after a process restart.
- A HITL pause can be rebuilt from session event history.
- Resume matches a response by interrupt ID and avoids re-consuming a completed
  handoff.
- Node input, output, and response can be schema-validated.

Two essential differences from Scope remain:

1. External calls from nodes, agents, and tools happen directly, with no
   unified effect identity, pending and settled boundary, or replay contract.
2. `workflow.New` still carries an explicit graph-fingerprint TODO in the
   source. When a workflow of the same name recovers after the graph evolved,
   there is no identity check between historical run state and current
   topology.

So ADK already has graph-level pause and resume, but that does not imply every
external side effect replays exactly.

## The eight dimensions

| Dimension | ADK Go's actual trade-off | Key difference from Scope |
| --- | --- | --- |
| Protocol boundary | A unified agent, model, tool, and session surface with natural Google ecosystem integration | Scope emphasizes a vendor-neutral independent core |
| Minimal contract | Sealed agents; the workflow node contract is wider and allows schemas | Scope's Definition and Execution are narrower but freeze the recovery protocol at the entry point |
| State ownership | Session, InvocationContext, and workflow run state cooperate | Scope separates host product data, ExecutionState, and tree state |
| Side effects | Agents, nodes, and tools call directly at run time | A Scope Step performs no I/O, and Effects have a stable identity |
| Orchestration | The workflow graph plus legacy workflow agents | Scope Workflow expresses only closed stages backed by real Processes |
| Recovery | Durable run state in the session, history rebuild, HITL resume | Scope additionally persists effect phase, the whole Process tree, and an exact DeploymentRef |
| Extension | Callbacks, plugins, and session events | Scope prefers neutral middleware and listeners, with OpenTelemetry as a separate module |
| Dependencies | Framework, services, and Google protocols cooperate closely | Scope isolates leaf adapters more thoroughly |

## What Scope should learn

1. **Schema validation at workflow construction and run time.** An input and
   output contract should not live only in documentation.
2. **The HITL response protocol.** Interrupt ID, response schema, duplicate
   resume, and stale response all have explicit error semantics.
3. **A single writer for concurrent state.** Concurrent node execution with a
   single scheduler committing state matches Scope's tree-owner principle.
4. **Rebuilding paused state from event history.** Even when the canonical
   snapshot is the source of truth, events can still serve diagnosis and
   cross-checking.
5. **Graph ergonomics.** Functions, tools, agents, and sub-workflows can all be
   nodes; ordinary graph composition is lighter than one Process per node.

## What Scope should not copy

- Do not recentralize every service and application state into a wide
  invocation context.
- Do not keep two semantically overlapping workflow APIs stable indefinitely;
  ADK's current coexistence is itself a migration and comprehension cost.
- Do not allow a durable topology to lack an exact definition or graph
  identity; Scope's DeploymentRef and snapshot validation must stay.
- Do not substitute session event replay for effect-level settlement and
  idempotency adjudication.
- Do not let Google-specific services into a core protocol for ecosystem
  convenience.

## Final placement

ADK Go is already a session agent runtime plus a graph workflow framework, not
just a few sequential and parallel agents. It is considerably stronger at graph
composition, HITL, and session-driven resume than an earlier draft described.
Scope keeps stricter recovery semantics for external side-effect identity, the
complete execution tree, and deployment version consistency.
