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

import { EventType, type BaseEvent, type CustomEvent } from "@ag-ui/core";
import {
  lookupCoreEventHandlers,
  lookupCustomEventHandler,
  reportPluginError,
  usePluginStore,
} from "@/plugins/sdk";
import { INITIAL_VIEW_STATE, type AgentViewState } from "./viewState";

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

// Hand a CUSTOM event to the plugin registry. If a plugin has registered a
// handler for ev.name, its returned StateUpdate is applied; otherwise the
// event is silently ignored.
function applyCustom(state: AgentViewState, ev: CustomEvent): AgentViewState {
  const handler = lookupCustomEventHandler(ev.name);
  if (!handler) return state;
  try {
    const update = handler(ev.value);
    return typeof update === "function" ? update(state) : state;
  } catch (err) {
     
    console.error(`[plugin] agui handler "${ev.name}" threw:`, err);
    const owner =
      usePluginStore.getState().customEventHandlers.get(ev.name)?.pluginName ?? "unknown";
    reportPluginError(owner, "agui", err, `event: ${ev.name}`);
    return state;
  }
}

export function reduce(state: AgentViewState, ev: BaseEvent): AgentViewState {
  if (ev.type === EventType.CUSTOM) return applyCustom(state, ev as CustomEvent);
  return applyCoreHandlers(state, ev);
}

export function reduceAll(events: Iterable<BaseEvent>): AgentViewState {
  let s = INITIAL_VIEW_STATE;
  for (const ev of events) s = reduce(s, ev);
  return s;
}
