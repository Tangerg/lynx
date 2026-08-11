import type { Item, RunEvent } from "@/rpc";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { measureReduce } from "@/lib/metrics";
import { lookupCustomHandlers, lookupStreamHandlers, reportPluginError } from "@/plugins/sdk";
import { onItemCompleted, onItemStarted } from "./itemHandlers";
import { durableItemSource } from "./source";

function applyStreamHandlers(state: AgentSessionView, event: RunEvent): AgentSessionView {
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

function applyCustom(state: AgentSessionView, event: RunEvent): AgentSessionView {
  if (event.event.type !== "custom") return state;
  const handlers = lookupCustomHandlers(event.event.name);
  if (handlers.length === 0) return state;
  let next = state;
  for (const { pluginName, handler } of handlers) {
    try {
      const update = handler(event.event.payload);
      if (typeof update === "function") next = update(next);
    } catch (error) {
      console.error(`[plugin] custom handler "${event.event.name}" (${pluginName}) threw:`, error);
      reportPluginError(pluginName, "events", error, `event: ${event.event.name}`);
    }
  }
  return next;
}

export function reduceRunEvent(state: AgentSessionView, event: RunEvent): AgentSessionView {
  const tag = event.event.type === "custom" ? event.event.name : event.event.type;
  return measureReduce(tag, () =>
    event.event.type === "custom" ? applyCustom(state, event) : applyStreamHandlers(state, event),
  );
}

export function reduceDurableItem(state: AgentSessionView, item: Item): AgentSessionView {
  const source = durableItemSource(item);
  return item.status === "running"
    ? onItemStarted(state, item, source)
    : onItemCompleted(state, item, source);
}
