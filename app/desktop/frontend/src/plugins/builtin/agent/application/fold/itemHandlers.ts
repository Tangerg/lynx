import type { Item, ItemDelta } from "@/rpc";
import type { AgentSessionView, TimelineEntry } from "@/plugins/sdk/types/agentSessionView";
import { appendTimelineEntry, setRunPlan } from "@/plugins/sdk";
import { blockStatus, mapPlan } from "./projections";
import {
  appendUserMessage,
  foldCompaction,
  foldQuestion,
  foldReasoning,
  foldText,
  patchRunBlock,
  updateTool,
  writeToolCall,
} from "./fold";
import type { AgentFoldSource } from "./source";
import { sourceTimestamp } from "./source";

function assertItemSource(item: Item, source: AgentFoldSource): void {
  if (item.runId !== source.runId) {
    throw new Error(
      `agent.fold.itemSourceMismatch:type=${item.type};item=${item.id};itemRun=${item.runId};eventRun=${source.runId}`,
    );
  }
}

function toolTimelineEntry(
  source: AgentFoldSource,
  kind: "tool-start" | "tool-end",
  patch: Pick<TimelineEntry, "refId" | "summary"> & Partial<Pick<TimelineEntry, "status">>,
): TimelineEntry {
  return {
    id: `timeline:${source.eventId}:${kind}`,
    ts: sourceTimestamp(source),
    kind,
    runId: source.runId,
    ...patch,
  };
}

export function onItemStarted(
  state: AgentSessionView,
  item: Item,
  source: AgentFoldSource,
): AgentSessionView {
  assertItemSource(item, source);
  switch (item.type) {
    case "userMessage":
      return appendUserMessage(state, item);
    case "agentMessage":
      return foldText(state, item, blockStatus(item.status));
    case "reasoning":
      return foldReasoning(state, item, blockStatus(item.status));
    case "toolCall": {
      const { state: next, tool } = writeToolCall(state, item);
      return appendTimelineEntry(
        toolTimelineEntry(source, "tool-start", { refId: item.id, summary: tool.fn }),
      )(next);
    }
    case "question":
      return foldQuestion(state, item, blockStatus(item.status));
    case "plan":
      return setRunPlan(item.runId, mapPlan(item.steps))(state);
    case "compaction":
      return foldCompaction(state, item);
  }
}

export function onItemDelta(
  state: AgentSessionView,
  itemId: string,
  delta: ItemDelta,
  source: AgentFoldSource,
): AgentSessionView {
  switch (delta.type) {
    case "content":
      return patchRunBlock(
        state,
        source.runId,
        (block) => block.kind === "text" && block.itemId === itemId,
        (block) => (block.kind === "text" ? { ...block, text: block.text + delta.text } : block),
      );
    case "reasoning":
      return patchRunBlock(
        state,
        source.runId,
        (block) => block.kind === "reasoning" && block.reasoningId === itemId,
        (block) =>
          block.kind === "reasoning" ? { ...block, text: block.text + delta.text } : block,
      );
    case "toolArguments":
      return updateTool(state, source.runId, itemId, (tool) => ({
        ...tool,
        args: tool.args + delta.argumentsTextDelta,
      }));
    case "toolOutput":
      return updateTool(state, source.runId, itemId, (tool) => ({
        ...tool,
        result: (tool.result ?? "") + delta.text,
      }));
    case "plan":
      return setRunPlan(source.runId, mapPlan(delta.steps))(state);
  }
}

export function onItemCompleted(
  state: AgentSessionView,
  rawItem: Item,
  source: AgentFoldSource,
): AgentSessionView {
  assertItemSource(rawItem, source);
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
      return appendTimelineEntry(
        toolTimelineEntry(source, "tool-end", {
          refId: item.id,
          status: tool.status === "err" ? "err" : tool.status === "denied" ? "declined" : "ok",
          summary: tool.fn,
        }),
      )(next);
    }
    case "question":
      return foldQuestion(state, item, blockStatus(item.status));
    case "plan":
      return setRunPlan(item.runId, mapPlan(item.steps))(state);
    case "compaction":
      return foldCompaction(state, item);
  }
}
