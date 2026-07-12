// Run-event handler types. The reducer is a pure
// dispatcher: it routes each v2 `StreamEvent` to the plugin handlers
// registered via `host.events.onStream(type, …)` (first-class events:
// run.* / item.* / state.*) and `host.events.onCustom(name, …)` (the
// `custom` StreamEvent, third-party extension only). The built-in protocol
// semantics live in `lyra.builtin.agent-fold`.

import type { StreamEvent } from "@/rpc";
import type { AgentViewState } from "@/plugins/sdk/types/agentView";

/**
 * Pure state update — takes the current view state, returns the next.
 *
 * Handlers compose updates from helpers exported by `@/plugins/sdk/state`
 * (e.g. `appendBlockToMessage`) so they don't have to know the state shape.
 */
export type StateUpdate = (state: AgentViewState) => AgentViewState;

/**
 * `custom` StreamEvent handler. Receives the event's `payload` and returns
 * either a StateUpdate (to mutate the view state) or void (for
 * side-effect-only handlers like analytics). For third-party extensions —
 * the runtime's own behaviour uses first-class event/Item types, never
 * `custom` (API.md §2.3).
 */
export type CustomEventHandler<T = unknown> = (value: T) => StateUpdate | void;

/**
 * Handler for a first-class StreamEvent type (run.started / run.finished /
 * item.started / item.delta / item.completed / state.snapshot / state.delta).
 * Receives the full state + the StreamEvent and returns the next state.
 * Multiple plugins can register for the same type; they run in registration
 * order, each seeing the previous output.
 *
 * Pluginifying these makes "everything is a plugin" literal: even the v2
 * protocol fold is just one (replaceable) plugin's contribution.
 *
 * `runId` is the wire (RunEvent envelope) runId that carried this event —
 * threaded through so run.* handlers can tell a subagent's run from the root's
 * (RunOutcome itself carries no id). `segmentId` is the envelope segmentId —
 * the streamed segment; a change in it is the segment boundary that resets the
 * per-segment streaming readout (a resume opens a new segment of the same run).
 * Both are absent for synthetic events (the optimistic local bubble, items.list
 * history replay).
 */
export type StreamEventHandler = (
  state: AgentViewState,
  event: StreamEvent,
  runId?: string,
  segmentId?: string,
) => AgentViewState;
