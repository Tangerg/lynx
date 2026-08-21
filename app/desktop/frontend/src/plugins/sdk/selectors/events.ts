// StreamEvent handler lookups — used imperatively by the reducer at dispatch
// time. Both surfaces are O(1) per lookup thanks to the cached per-point index
// (createPointSubIndex, which invalidates when the point's contributions do).

import type { StreamEventHandler } from "../types";
import { STREAM_EVENT_HANDLER } from "../kernelPoints";
import { contributionsTo } from "../kernel";
import { createPointSubIndex } from "./extensions";

type StreamHandlerItem = { eventType: string; handler: StreamEventHandler };

const coreByType = createPointSubIndex((item: StreamHandlerItem, pluginName) => ({
  key: item.eventType,
  value: { pluginName, handler: item.handler },
}));

/**
 * Every *core* handler registered for a built-in StreamEvent type. Insertion
 * order; the reducer chains them through the state.
 */
export function lookupStreamHandlers(
  eventType: string,
): Array<{ pluginName: string; handler: StreamEventHandler }> {
  return coreByType(contributionsTo(STREAM_EVENT_HANDLER)).get(eventType) ?? [];
}
