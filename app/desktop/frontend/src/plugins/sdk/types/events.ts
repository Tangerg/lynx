// Run-event handler types. The reducer is a pure
// dispatcher: it routes each v2 `StreamEvent` to the plugin handlers
// registered for first-class run.* / item.* / state.* events. The built-in
// protocol semantics live in `lyra.builtin.agent-fold`.

import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import type { AgentEventEnvelope } from "./agentEvents";

/**
 * Pure state update — takes the current view state, returns the next.
 *
 * Handlers compose updates from helpers exported by `@/plugins/sdk/state`
 * (e.g. `appendBlockToMessage`) so they don't have to know the state shape.
 */
export type StateUpdate = (state: AgentSessionView) => AgentSessionView;

/**
 * Handler for a first-class StreamEvent type (segment.started / segment.finished /
 * item.started / item.delta / item.completed / state.snapshot).
 * Receives the full session projection + the complete RunEvent envelope and
 * returns the next projection.
 * Multiple plugins can register for the same type; they run in registration
 * order, each seeing the previous output.
 *
 * Pluginifying these makes "everything is a plugin" literal: even the v2
 * protocol fold is just one (replaceable) plugin's contribution.
 *
 * The envelope is mandatory provenance: source Run, Segment, event identity and
 * runtime timestamp cannot be reconstructed from a payload or current UI state.
 */
export type StreamEventHandler = (
  state: AgentSessionView,
  event: AgentEventEnvelope,
) => AgentSessionView;
