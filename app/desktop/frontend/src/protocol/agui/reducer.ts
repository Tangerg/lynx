// Fold a stream of AG-UI events (@ag-ui/core BaseEvents) into AgentViewState.
//
// This file is intentionally a *pure dispatcher* — every reducer case
// lives in a plugin. Even the built-in event types (RUN_*, TEXT_MESSAGE_*,
// TOOL_CALL_*, REASONING_*) are owned by `lyra.builtin.core-reducer`.
// The reducer here only:
//
//   1. for built-in event types — chains through every plugin registered
//      via `host.agui.onCore(type, handler)`
//   2. for CUSTOM events — routes to plugins via `host.agui.on(name, handler)`
//
// Both pathways use the same error-isolation policy: a throwing handler is
// logged to the error store and its input state is preserved.

import type { BaseEvent, CustomEvent } from "@ag-ui/core";
import type { AgentViewState } from "./viewState";
import { EventType } from "@ag-ui/core";
import { measureReduce } from "@/lib/metrics";
import {
  lookupCoreEventHandlers,
  lookupCustomEventHandlers,
  reportPluginError,
} from "@/plugins/sdk";

function applyCoreHandlers(state: AgentViewState, event: BaseEvent): AgentViewState {
  const handlers = lookupCoreEventHandlers(event.type);
  if (handlers.length === 0) return state;
  let next = state;
  for (const { pluginName, handler } of handlers) {
    try {
      next = handler(next, event);
    } catch (err) {
      console.error(`[plugin] core handler "${event.type}" (${pluginName}) threw:`, err);
      reportPluginError(pluginName, "agui", err, `event: ${event.type}`);
      // Skip this handler — keep `next` as it was before the throw so the
      // rest of the chain still gets a chance to run.
    }
  }
  return next;
}

// Fan a CUSTOM event out through every plugin registered for `ev.name`.
// Each handler returns an optional StateUpdate (`(state) => state`); the
// reducer threads state through the chain in registration order. A throwing
// handler is logged + reported, then skipped — the rest of the chain still
// runs, same isolation policy as core handlers.
function applyCustom(state: AgentViewState, ev: CustomEvent): AgentViewState {
  const handlers = lookupCustomEventHandlers(ev.name);
  if (handlers.length === 0) return state;
  let next = state;
  for (const { pluginName, handler } of handlers) {
    try {
      const update = handler(ev.value);
      if (typeof update === "function") next = update(next);
    } catch (err) {
      console.error(`[plugin] agui handler "${ev.name}" (${pluginName}) threw:`, err);
      reportPluginError(pluginName, "agui", err, `event: ${ev.name}`);
    }
  }
  return next;
}

export function reduce(state: AgentViewState, ev: BaseEvent): AgentViewState {
  const isCustom = ev.type === EventType.CUSTOM;
  // CUSTOM events carry the discriminating name in `ev.name`; core
  // events use `ev.type`. Tag the metric with the most specific
  // discriminator either side has.
  const tag = isCustom ? (ev as CustomEvent).name : ev.type;
  return measureReduce(tag, () =>
    isCustom ? applyCustom(state, ev as CustomEvent) : applyCoreHandlers(state, ev),
  );
}
