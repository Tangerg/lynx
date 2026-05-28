// AG-UI event handler lookups — used imperatively by the reducer at
// dispatch time. Both surfaces are O(1) per lookup thanks to the cached
// secondary index in _helpers (invalidates on registry mutation).

import type { CoreEventHandler, CustomEventHandler } from "../types";
import { usePluginStore } from "../registry";
import { createIndex } from "./_helpers";

const customByName = createIndex<
  { name: string; handler: CustomEventHandler<unknown> },
  { pluginName: string; handler: CustomEventHandler<unknown> }
>((o) => ({
  key: o.value.name,
  value: { pluginName: o.pluginName, handler: o.value.handler },
}));

const coreByType = createIndex<
  { eventType: string; handler: CoreEventHandler },
  { pluginName: string; handler: CoreEventHandler }
>((o) => ({
  key: o.value.eventType,
  value: { pluginName: o.pluginName, handler: o.value.handler },
}));

/**
 * Look up every CUSTOM-event handler registered for `name`, in registration
 * order. The reducer fans the event out through all of them, chaining each
 * handler's StateUpdate return through the state.
 */
export function lookupCustomEventHandlers(
  name: string,
): Array<{ pluginName: string; handler: CustomEventHandler<unknown> }> {
  return customByName(usePluginStore.getState().customEventHandlers).get(name) ?? [];
}

/**
 * Look up all *core* handlers registered for an AG-UI built-in event type.
 * Returned in insertion order; the reducer chains them through the state.
 */
export function lookupCoreEventHandlers(
  eventType: string,
): Array<{ pluginName: string; handler: CoreEventHandler }> {
  return coreByType(usePluginStore.getState().coreEventHandlers).get(eventType) ?? [];
}
