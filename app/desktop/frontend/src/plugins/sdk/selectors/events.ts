// StreamEvent handler lookups — used imperatively by the reducer at dispatch
// time. Both surfaces are O(1) per lookup thanks to the cached per-point index
// (createPointSubIndex, which invalidates when the point's contributions do).

import type { StreamEventHandler, CustomEventHandler } from "../types";
import { STREAM_EVENT_HANDLER, CUSTOM_EVENT_HANDLER } from "../kernelPoints";
import { contributionsTo } from "../kernel";
import { createPointSubIndex } from "./extensions";

type CustomHandlerItem = { name: string; handler: CustomEventHandler<unknown> };
type StreamHandlerItem = { eventType: string; handler: StreamEventHandler };

const customByName = createPointSubIndex((item: CustomHandlerItem, pluginName) => ({
  key: item.name,
  value: { pluginName, handler: item.handler },
}));

const coreByType = createPointSubIndex((item: StreamHandlerItem, pluginName) => ({
  key: item.eventType,
  value: { pluginName, handler: item.handler },
}));

/**
 * Every CUSTOM-event handler registered for `name`, in registration order. The
 * reducer fans the event out through all of them, chaining each handler's
 * StateUpdate return through the state.
 */
export function lookupCustomHandlers(
  name: string,
): Array<{ pluginName: string; handler: CustomEventHandler<unknown> }> {
  return customByName(contributionsTo(CUSTOM_EVENT_HANDLER)).get(name) ?? [];
}

/**
 * Every *core* handler registered for a built-in StreamEvent type. Insertion
 * order; the reducer chains them through the state.
 */
export function lookupStreamHandlers(
  eventType: string,
): Array<{ pluginName: string; handler: StreamEventHandler }> {
  return coreByType(contributionsTo(STREAM_EVENT_HANDLER)).get(eventType) ?? [];
}
