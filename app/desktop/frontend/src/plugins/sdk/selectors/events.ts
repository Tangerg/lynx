// AG-UI event handler lookups — used imperatively by the reducer at
// dispatch time. Both surfaces are O(1) per lookup thanks to the cached
// secondary index in _helpers (invalidates on registry mutation).

import type { StreamEventHandler, CustomEventHandler } from "../types";
import { STREAM_EVENT_HANDLER, CUSTOM_EVENT_HANDLER } from "../kernelPoints";
import { usePluginStore } from "../registry";
import { createPointSubIndex } from "./extensions";

const customByName = createPointSubIndex<
  { name: string; handler: CustomEventHandler<unknown> },
  { pluginName: string; handler: CustomEventHandler<unknown> }
>(CUSTOM_EVENT_HANDLER.id, (item, pluginName) => ({
  key: item.name,
  value: { pluginName, handler: item.handler },
}));

const coreByType = createPointSubIndex<
  { eventType: string; handler: StreamEventHandler },
  { pluginName: string; handler: StreamEventHandler }
>(STREAM_EVENT_HANDLER.id, (item, pluginName) => ({
  key: item.eventType,
  value: { pluginName, handler: item.handler },
}));

/**
 * Look up every CUSTOM-event handler registered for `name`, in registration
 * order. The reducer fans the event out through all of them, chaining each
 * handler's StateUpdate return through the state.
 */
export function lookupCustomHandlers(
  name: string,
): Array<{ pluginName: string; handler: CustomEventHandler<unknown> }> {
  return customByName(usePluginStore.getState().extensions).get(name) ?? [];
}

/**
 * Look up all *core* handlers registered for an AG-UI built-in event type.
 * Returned in insertion order; the reducer chains them through the state.
 */
export function lookupStreamHandlers(
  eventType: string,
): Array<{ pluginName: string; handler: StreamEventHandler }> {
  return coreByType(usePluginStore.getState().extensions).get(eventType) ?? [];
}
