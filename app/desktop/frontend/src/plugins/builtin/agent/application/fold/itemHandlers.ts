import type { AgentItem, AgentItemDelta } from "@/plugins/sdk";
import type { AgentSessionView, TimelineEntry } from "@/plugins/sdk/types/agentSessionView";
import { appendTimelineEntry } from "@/plugins/sdk";
import { blockStatus } from "./projections";
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

function assertItemSource(item: AgentItem, source: AgentFoldSource): void {
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
  item: AgentItem,
  source: AgentFoldSource,
): AgentSessionView {
  assertItemSource(item, source);
  // item.started is a create-once observation. Replays may arrive after
  // deltas, item.completed, an interrupt materialization, or a durable
  // snapshot has already advanced the same Item. Letting the older shell
  // upsert at that point would erase content and regress complete/err cards
  // back to running. Only item.completed is allowed to reconcile an existing
  // projection with authoritative fields.
  const materialized = assertItemProjectionIdentity(state, item);
  if (
    item.type !== "userMessage" &&
    materialized &&
    !(item.type === "toolCall" && state.toolCalls[item.id]?.status === "requires-action")
  ) {
    return state;
  }
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
    case "compaction":
      return foldCompaction(state, item);
  }
}

interface ProjectedItemIdentity {
  type: AgentItem["type"];
  runId: string | null;
}

function assertItemProjectionIdentity(state: AgentSessionView, item: AgentItem): boolean {
  const identities: ProjectedItemIdentity[] = [];
  const tool = state.toolCalls[item.id];
  if (tool) identities.push({ type: "toolCall", runId: tool.runId });

  for (const message of state.messages) {
    if (message.id === item.id) {
      if (message.role === "user") {
        identities.push({ type: "userMessage", runId: message.runId });
      } else if (message.blocks.some((block) => block.kind === "compaction")) {
        identities.push({ type: "compaction", runId: message.runId });
      }
    }
    for (const block of message.blocks) {
      switch (block.kind) {
        case "text":
          if (block.itemId === item.id)
            identities.push({ type: "agentMessage", runId: message.runId });
          break;
        case "reasoning":
          if (block.reasoningId === item.id)
            identities.push({ type: "reasoning", runId: message.runId });
          break;
        case "tool":
          if (block.toolCallId === item.id)
            identities.push({ type: "toolCall", runId: message.runId });
          break;
        case "approval":
          if (block.itemId === item.id) identities.push({ type: "toolCall", runId: message.runId });
          break;
        case "question":
          if (block.itemId === item.id) identities.push({ type: "question", runId: message.runId });
          break;
        default:
          break;
      }
    }
  }

  for (const identity of identities) {
    if (identity.type !== item.type || (identity.runId !== null && identity.runId !== item.runId)) {
      throw new Error(
        `agent.fold.itemIdentityConflict:item=${item.id};itemType=${item.type};itemRun=${item.runId};existingType=${identity.type};existingRun=${identity.runId ?? "unbound"}`,
      );
    }
  }
  return identities.length > 0;
}

export function onItemDelta(
  state: AgentSessionView,
  itemId: string,
  delta: AgentItemDelta,
  source: AgentFoldSource,
): AgentSessionView {
  switch (delta.type) {
    case "content":
      return patchRunBlock(
        state,
        source.runId,
        (block) => block.kind === "text" && block.itemId === itemId && block.status === "running",
        (block) => (block.kind === "text" ? { ...block, text: block.text + delta.text } : block),
      );
    case "reasoning":
      return patchRunBlock(
        state,
        source.runId,
        (block) =>
          block.kind === "reasoning" && block.reasoningId === itemId && block.status === "running",
        (block) =>
          block.kind === "reasoning" ? { ...block, text: block.text + delta.text } : block,
      );
    case "toolArguments": {
      const tool = state.toolCalls[itemId];
      if (!tool || tool.runId !== source.runId || tool.status !== "running") return state;
      return updateTool(state, source.runId, itemId, (tool) => ({
        ...tool,
        args: tool.args + delta.argumentsTextDelta,
      }));
    }
    case "toolOutput": {
      const tool = state.toolCalls[itemId];
      if (!tool || tool.runId !== source.runId || tool.status !== "running") return state;
      return updateTool(state, source.runId, itemId, (tool) => ({
        ...tool,
        result: (tool.result ?? "") + delta.text,
      }));
    }
  }
}

export function onItemCompleted(
  state: AgentSessionView,
  item: AgentItem,
  source: AgentFoldSource,
): AgentSessionView {
  assertItemSource(item, source);
  assertItemProjectionIdentity(state, item);
  if (item.status === "running") {
    throw new Error(
      `agent.fold.itemCompletedRequiresTerminalStatus:item=${item.id};run=${item.runId};status=${item.status}`,
    );
  }
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
    case "compaction":
      return foldCompaction(state, item);
  }
}
