import type { Item, ItemDelta } from "@/rpc";
import type { AgentViewState } from "@/protocol/run/viewState";
import { appendTimelineEntry, setPlan } from "@/plugins/sdk";
import { blockStatus, mapPlan } from "./projections";
import {
  appendUserMessage,
  foldCompaction,
  foldQuestion,
  foldReasoning,
  foldText,
  patchBlock,
  updateTool,
  writeToolCall,
} from "./fold";

export function onItemStarted(state: AgentViewState, item: Item): AgentViewState {
  switch (item.type) {
    case "userMessage":
      return appendUserMessage(state, item);
    case "agentMessage":
      return foldText(state, item, blockStatus(item.status));
    case "reasoning":
      return foldReasoning(state, item, blockStatus(item.status));
    case "toolCall": {
      const { state: next, tool } = writeToolCall(state, item);
      return appendTimelineEntry({ kind: "tool-start", refId: item.id, summary: tool.fn })(next);
    }
    case "question":
      return foldQuestion(state, item, blockStatus(item.status));
    case "plan":
      return setPlan(mapPlan(item.steps))(state);
    case "compaction":
      return foldCompaction(state, item);
  }
}

export function onItemDelta(
  state: AgentViewState,
  itemId: string,
  delta: ItemDelta,
): AgentViewState {
  switch (delta.type) {
    case "content":
      return patchBlock(
        state,
        (b) => b.kind === "text" && b.itemId === itemId,
        (b) => (b.kind === "text" ? { ...b, text: b.text + delta.text } : b),
      );
    case "reasoning":
      return patchBlock(
        state,
        (b) => b.kind === "reasoning" && b.reasoningId === itemId,
        (b) => (b.kind === "reasoning" ? { ...b, text: b.text + delta.text } : b),
      );
    case "toolArguments":
      return updateTool(state, itemId, (t) => ({ ...t, args: t.args + delta.argumentsTextDelta }));
    case "toolOutput":
      return updateTool(state, itemId, (t) => ({ ...t, result: (t.result ?? "") + delta.text }));
    case "plan":
      return setPlan(mapPlan(delta.steps))(state);
  }
}

export function onItemCompleted(state: AgentViewState, rawItem: Item): AgentViewState {
  // A completed item with running status usually comes from crash/restart
  // history hydration. Render it as truncated instead of spinning forever.
  const item: Item = rawItem.status === "running" ? { ...rawItem, status: "incomplete" } : rawItem;
  switch (item.type) {
    case "userMessage":
      return appendUserMessage(state, item);
    case "agentMessage":
      return foldText(state, item, blockStatus(item.status));
    case "reasoning":
      return foldReasoning(state, item, blockStatus(item.status));
    case "toolCall": {
      const { state: next, tool } = writeToolCall(state, item);
      return appendTimelineEntry({
        kind: "tool-end",
        refId: item.id,
        status: tool.status === "err" ? "err" : tool.status === "denied" ? "declined" : "ok",
        summary: tool.fn,
      })(next);
    }
    case "question":
      return foldQuestion(state, item, blockStatus(item.status));
    case "plan":
      return setPlan(mapPlan(item.steps))(state);
    case "compaction":
      return foldCompaction(state, item);
  }
}
