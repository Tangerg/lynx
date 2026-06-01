// MESSAGES_SNAPSHOT — bulk hydration on reconnect / thread switch.
//
// Replaces messages + toolCalls wholesale (snapshot is authoritative);
// leaves run / plan / error so a mid-run UI keeps its stop button + plan.
// developer / system both collapse to "system"; role:"tool" messages fold
// into toolCalls[id].result (they're results, not chat turns). Snapshot
// tools default to "ok" + duration "—" since they represent settled
// history.

import type { MessagesSnapshotEvent } from "@ag-ui/core";
import type { AgentViewState, ContentBlock, Message, ToolCall } from "@/protocol/agui/viewState";
import { nameForRole, nowTime, roleFromSnapshotMessage } from "../helpers";

type SnapshotMessage = MessagesSnapshotEvent["messages"][number];
interface SnapToolCall {
  id: string;
  type: "function";
  function: { name: string; arguments: string };
}

// Tool result — attach to its tool call entry. If the matching tool call
// hasn't been seen yet (out-of-order snapshot), stash a minimal entry;
// the downstream assistant message will fill in fn.
function ingestToolResult(toolCalls: Record<string, ToolCall>, m: SnapshotMessage): void {
  const tcId = (m as { toolCallId: string }).toolCallId;
  const content = (m as { content: string }).content;
  const errored = Boolean((m as { error?: string }).error);
  const prev = toolCalls[tcId];
  toolCalls[tcId] = {
    id: tcId,
    fn: prev?.fn ?? "",
    args: prev?.args ?? "",
    status: errored ? "err" : "ok",
    duration: prev?.duration ?? "—",
    result: content,
  };
}

// Build assistant blocks (text + tool placeholders) and side-effect the
// accumulator with each tool call's metadata.
function buildAssistantBlocks(
  m: SnapshotMessage,
  toolCalls: Record<string, ToolCall>,
): ContentBlock[] {
  const blocks: ContentBlock[] = [];
  const content = (m as { content?: string }).content;
  if (content) {
    blocks.push({ kind: "text", text: content, status: "complete" });
  }
  const tcs = (m as { toolCalls?: SnapToolCall[] }).toolCalls ?? [];
  for (const tc of tcs) {
    blocks.push({ kind: "tool", toolCallId: tc.id });
    const prev = toolCalls[tc.id];
    toolCalls[tc.id] = {
      id: tc.id,
      fn: tc.function.name,
      args: tc.function.arguments,
      status: prev?.status ?? "ok",
      duration: prev?.duration ?? "—",
      result: prev?.result,
    };
  }
  return blocks;
}

export const onMessagesSnapshot = (
  state: AgentViewState,
  ev: MessagesSnapshotEvent,
): AgentViewState => {
  const messages: Message[] = [];
  const toolCalls: Record<string, ToolCall> = {};

  for (const m of ev.messages as SnapshotMessage[]) {
    if (m.role === "tool") {
      ingestToolResult(toolCalls, m);
      continue;
    }

    const role = roleFromSnapshotMessage(m.role);

    const blocks: ContentBlock[] =
      m.role === "assistant"
        ? buildAssistantBlocks(m, toolCalls)
        : [{ kind: "text", text: (m as { content: string }).content, status: "complete" }];

    messages.push({
      id: m.id,
      role,
      who: nameForRole(role),
      // Snapshot messages don't carry timestamps in the AG-UI schema — we
      // use "now" as a stand-in. Real backends should include timestamps.
      time: nowTime(),
      blocks,
    });
  }

  return { ...state, messages, toolCalls };
};
