# Pi: a compact tool loop and an unfinished durable harness

Evidence baseline: `pi` commit
`853a80d26c90a14c1886f0ebb8ffaae133ca2185`.

## Framework-level judgment

Pi is not a monolithic "coding agent framework". The repository should be read
as at least three layers:

- `pi-ai`: a unified multi-provider model interface.
- `pi-agent-core`: the model-tool loop, a stateful agent, and a harness under
  construction.
- `pi-coding-agent`: an interactive coding application.

This round compares the first two layers only. The coding agent's terminal,
editor, session UI, and built-in tools are application concerns and do not
enter the framework kernel conclusion.

What Pi has already delivered is strongest at forming an embeddable,
multi-provider tool loop with lifecycle callbacks behind a very small
interface. Its core goal differs from Scope's: Pi optimizes direct execution
and interactive productivity first, Scope optimizes managed side effects and
interruption recovery first.

## The actual core

### The model layer

`packages/ai/src/types.ts` defines a unified `Model` along with messages, tool
calls, stream events, and request options; `packages/ai/src/models.ts` provides
a provider registry and a dynamic model catalog.

This layer's strength is a uniform calling experience: thinking, tool calls,
usage, and errors all express through one stream protocol. The cost is that
public types retain several provider-specific fields, and the `pi-ai` package
depends directly on the Anthropic, Bedrock, Google, and OpenAI SDKs. It is a
pragmatic unification layer, not a fully vendor-neutral domain kernel.

### The low-level agent loop

The key abstractions in `packages/agent/src/types.ts` and `agent-loop.ts`:

- an injected `StreamFn` that decouples the model implementation from the loop;
- `AgentLoopConfig`, allowing message transformation, context trimming, key
  resolution, stop conditions, next-turn preparation, and pre- and post-tool
  callbacks;
- TypeBox tool schemas supporting sequential or parallel execution;
- steering and follow-up queues;
- an extensible `AgentMessage`, which applications extend through TypeScript
  declaration merging.

The loop calls models and tools directly, updates the transcript, and emits
progress as events. It also refuses to execute a tool call whose arguments were
truncated — a truthfulness and safety boundary worth keeping.

### The stateful agent

`packages/agent/src/agent.ts` adds state, event subscription, active run
control, and a message queue around the low-level loop. `AgentState` holds
messages and tools together with interaction state such as the streaming
message, pending tool calls, and errors.

That makes it very easy for an interactive application to subscribe to complete
state, but "persistable domain state" and "transient run or UI state" are not
naturally separated the way Scope separates host from execution.

## Harness: a design direction is not a delivered capability

`packages/agent/src/harness/agent-harness.ts` and its related types already
describe a markedly stronger run model:

- an operation log and operation outcomes;
- lanes, branches, and session snapshots;
- records such as `OperationStarted`, `StepAttempt`, `ToolStarted`,
  `OperationFinished`, and `UsageRecord`;
- a `never | safe` tool replay policy;
- JSONL, SQLite, and in-memory store interfaces;
- resume, compaction, tree navigation, queued operations, and actions.

These types show the Pi team already recognizes that the low-level loop cannot
carry durable execution and replay. But at this commit, the `create` recovery
path throws `HarnessNotImplemented` when records exist, and `prompt`, `resume`,
`compact`, tree navigation, and several operation APIs still take unimplemented
branches.

So this round's conclusion must be stated in two sentences:

1. Pi has a clear durable-harness design direction worth watching.
2. Pi's currently dependable capability is still the low-level direct execution
   loop; the harness type surface cannot be counted as a complete recovery
   implementation.

## The eight dimensions

| Dimension | Pi's actual trade-off | Key difference from Scope |
| --- | --- | --- |
| Protocol boundary | A unified multi-provider experience, with public types accommodating provider features | Scope's core is purer; Pi's adaptation experience is more direct |
| Minimal contract | A `StreamFn` plus tools is enough to run | Pi is lighter; Scope demands recovery semantics from the entry point |
| State ownership | The agent holds the transcript and running state | Scope leaves product state to the host and execution state to the Execution |
| Side effects | The loop calls models and tools directly | Scope describes an Effect first, then the runtime executes it |
| Orchestration | The low level is mainly a continuous tool loop | Scope has managed child Processes and Workflow |
| Recovery | No complete durable recovery at the low level; the harness is unfinished | Scope's snapshot and restore are part of the current kernel |
| Extension | Events, named hooks, its own telemetry schema | Scope prefers middleware and listeners plus an OpenTelemetry adapter |
| Package boundary | Separate AI, agent, and coding app packages; agent-core's export surface is wide | Scope isolates leaf modules more finely, and governs them at higher cost |

## What Scope should learn

1. **Make the simple path a genuine first-class entry point.** Pi proves that
   injecting a model stream function and tools yields a high-quality loop
   without first understanding a complete runtime.
2. **A message extension mechanism.** Application messages and model messages
   can be layered and converted before reaching the model, keeping product
   events out of the base model protocol.
3. **Clear stream events.** Model deltas, tool start and end, turn start and
   end, and errors are all directly consumable by the caller.
4. **Do not execute truncated arguments.** Refusing a side effect when the
   protocol is incomplete is more reliable than guessing or executing
   tolerantly.
5. **Make replay policy explicit.** The harness's `never | safe`, though not
   fully landed, is more honest than assuming every tool is replayable.

## What Scope should not copy

1. Do not let provider SDKs or proprietary fields flow back into `core`.
2. Do not add UI-adjacent state to a persisted Execution snapshot.
3. Do not give up effect identity for a simpler API; recovery semantics for
   long-running tasks are exactly Scope's design center.
4. Do not widen an unimplemented durable API into a stable public surface
   ahead of time.
5. Do not let coding-scenario search, sessions, and tool collections creep into
   a general agent kernel.

## Final placement

Pi is one of the most valuable new comparisons for Scope, because what it
reveals is not "which features are missing" but a different standard of
framework success: the shortest usable path, very strong interaction events,
and pragmatic provider unification.

Scope has more complete managed recovery semantics than Pi's current low-level
runtime; Pi has a more natural embedding experience for a short tool loop. The
two should correct each other rather than be flattened into one score.
