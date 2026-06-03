// Fold a stream of v2 RunEvents (StreamEvent payloads, API.md §5) into
// AgentViewState.
//
// This file is intentionally a *pure dispatcher* — every reducer case lives
// in a plugin. Even the built-in event types (run.* / item.* / state.*) are
// owned by `lyra.builtin.core-reducer`. The reducer here only:
//
//   1. for first-class StreamEvent types — chains through every plugin
//      registered via `host.events.onStream(type, handler)`;
//   2. for `custom` StreamEvents — routes to plugins via
//      `host.events.onCustom(name, handler)`.
//
// Both pathways use the same error-isolation policy: a throwing handler is
// logged to the error store and its input state is preserved.

import type { StreamEvent } from "@/rpc";
import type { AgentViewState } from "./viewState";
import { measureReduce } from "@/lib/metrics";
import { lookupCustomHandlers, lookupStreamHandlers, reportPluginError } from "@/plugins/sdk";

function applyStreamHandlers(state: AgentViewState, event: StreamEvent): AgentViewState {
  const handlers = lookupStreamHandlers(event.type);
  if (handlers.length === 0) return state;
  let next = state;
  for (const { pluginName, handler } of handlers) {
    try {
      next = handler(next, event);
    } catch (err) {
      console.error(`[plugin] stream handler "${event.type}" (${pluginName}) threw:`, err);
      reportPluginError(pluginName, "events", err, `event: ${event.type}`);
      // Skip this handler — keep `next` as it was before the throw so the
      // rest of the chain still gets a chance to run.
    }
  }
  return next;
}

// Fan a `custom` StreamEvent out through every plugin registered for its
// `name`. Each handler returns an optional StateUpdate (`(state) => state`);
// the reducer threads state through the chain in registration order. A
// throwing handler is logged + reported, then skipped — same isolation
// policy as stream handlers.
function applyCustom(
  state: AgentViewState,
  ev: Extract<StreamEvent, { type: "custom" }>,
): AgentViewState {
  const handlers = lookupCustomHandlers(ev.name);
  if (handlers.length === 0) return state;
  let next = state;
  for (const { pluginName, handler } of handlers) {
    try {
      const update = handler(ev.payload);
      if (typeof update === "function") next = update(next);
    } catch (err) {
      console.error(`[plugin] custom handler "${ev.name}" (${pluginName}) threw:`, err);
      reportPluginError(pluginName, "events", err, `event: ${ev.name}`);
    }
  }
  return next;
}

export function reduce(state: AgentViewState, ev: StreamEvent): AgentViewState {
  // `custom` events carry the discriminating name; first-class events use
  // `type`. Tag the metric with the most specific discriminator.
  const tag = ev.type === "custom" ? ev.name : ev.type;
  return measureReduce(tag, () =>
    ev.type === "custom" ? applyCustom(state, ev) : applyStreamHandlers(state, ev),
  );
}
