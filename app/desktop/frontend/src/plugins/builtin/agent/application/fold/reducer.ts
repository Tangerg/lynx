import type { AgentEventEnvelope, AgentItem } from "@/plugins/sdk";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { measureReduce } from "@/lib/metrics";
import { lookupStreamHandlers, reportPluginError } from "@/plugins/sdk";
import { onItemCompleted, onItemStarted } from "./itemHandlers";
import { durableItemSource } from "./source";

function applyStreamHandlers(state: AgentSessionView, event: AgentEventEnvelope): AgentSessionView {
  const handlers = lookupStreamHandlers(event.event.type);
  if (handlers.length === 0) return state;
  let next = state;
  for (const { pluginName, handler } of handlers) {
    try {
      next = handler(next, event);
    } catch (error) {
      console.error(`[plugin] stream handler "${event.event.type}" (${pluginName}) threw:`, error);
      reportPluginError(pluginName, "events", error, `event: ${event.event.type}`);
    }
  }
  return next;
}

export function reduceAgentEvent(
  state: AgentSessionView,
  event: AgentEventEnvelope,
): AgentSessionView {
  return measureReduce(event.event.type, () => applyStreamHandlers(state, event));
}

export function reduceDurableItem(state: AgentSessionView, item: AgentItem): AgentSessionView {
  const source = durableItemSource(item);
  return item.status === "running"
    ? onItemStarted(state, item, source)
    : onItemCompleted(state, item, source);
}
