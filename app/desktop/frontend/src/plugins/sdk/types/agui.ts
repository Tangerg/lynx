// AG-UI event handlers — both CUSTOM events and built-in core events
// (RUN_STARTED, TEXT_MESSAGE_*, TOOL_CALL_*, REASONING_*, …).

import type { BaseEvent } from "@ag-ui/core";
import type { AgentViewState } from "@/protocol/agui/viewState";

/**
 * Pure state update — takes the current view state, returns the next.
 *
 * Handlers compose updates from helpers exported by `@/plugins/sdk/state`
 * (e.g. `appendBlockToMessage`) so they don't have to know the state shape.
 */
export type StateUpdate = (state: AgentViewState) => AgentViewState;

/**
 * AG-UI CUSTOM event handler. Receives the event's `value` payload and
 * returns either a StateUpdate (to mutate the view state) or void (for
 * side-effect-only handlers like analytics).
 */
export type CustomEventHandler<T = unknown> = (value: T) => StateUpdate | void;

/**
 * Handler for an AG-UI *built-in* event type (RUN_STARTED, TEXT_MESSAGE_*,
 * TOOL_CALL_*, REASONING_*, etc.). Receives the full state + raw event and
 * returns the next state. Multiple plugins can register for the same event
 * type; they run in registration order, each seeing the previous output.
 *
 * The core protocol semantics live in the `lyra.builtin.core-reducer` plugin
 * — pluginifying these makes "everything is a plugin" literal: even the AG-UI
 * spec is just one (replaceable) plugin's contribution.
 */
export type CoreEventHandler = (state: AgentViewState, event: BaseEvent) => AgentViewState;
