// Run-event handler types. The reducer is a pure
// dispatcher: it routes each v2 `StreamEvent` to the plugin handlers
// registered for first-class run.* / item.* / plan.* events. The built-in
// protocol semantics live in `lyra.builtin.agent-fold`.

import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import type { AgentEventEnvelope } from "./agentEvents";

/**
 * Handler for a first-class StreamEvent type (segment.started / segment.finished /
 * item.started / item.delta / item.completed / plan.updated).
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
