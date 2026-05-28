// Tool-call stream handlers — TOOL_CALL_START / ARGS / END / RESULT,
// plus the fused TOOL_CALL_CHUNK variant.

import type {
  ToolCallArgsEvent,
  ToolCallChunkEvent,
  ToolCallEndEvent,
  ToolCallResultEvent,
  ToolCallStartEvent,
} from "@ag-ui/core";
import type { AgentViewState, ToolCall } from "@/protocol/agui/viewState";
import { appendBlock, appendTimeline, updateMessage, updateTool } from "../helpers";

export const onToolStart = (state: AgentViewState, ev: ToolCallStartEvent): AgentViewState => {
  const tool: ToolCall = {
    id: ev.toolCallId,
    fn: ev.toolCallName,
    args: "",
    status: "running",
    duration: "LIVE",
  };
  let next: AgentViewState = {
    ...state,
    toolCalls: { ...state.toolCalls, [ev.toolCallId]: tool },
  };
  if (ev.parentMessageId) {
    next = updateMessage(next, ev.parentMessageId, (m) =>
      appendBlock(m, { kind: "tool", toolCallId: ev.toolCallId }),
    );
  }
  return appendTimeline(next, {
    kind: "tool-start",
    summary: ev.toolCallName,
    refId: ev.toolCallId,
  });
};

export const onToolArgs = (state: AgentViewState, ev: ToolCallArgsEvent): AgentViewState =>
  updateTool(state, ev.toolCallId, (t) => ({ ...t, args: t.args + ev.delta }));

export const onToolEnd = (state: AgentViewState, ev: ToolCallEndEvent): AgentViewState => {
  // AG-UI's TOOL_CALL_END is minimal — most "result" data rides on
  // TOOL_CALL_RESULT or comes via summary fields we surface as extras.
  const ex = ev as ToolCallEndEvent & {
    status?: ToolCall["status"];
    durationMs?: number;
    added?: number;
    removed?: number;
    hits?: number;
    lines?: number;
  };
  const finalStatus = ex.status ?? "ok";
  const next = updateTool(state, ev.toolCallId, (t) => ({
    ...t,
    status: finalStatus,
    duration:
      ex.durationMs != null ? `${ex.durationMs}ms` : t.duration === "LIVE" ? "—" : t.duration,
    added: ex.added ?? t.added,
    removed: ex.removed ?? t.removed,
    hits: ex.hits ?? t.hits,
    lines: ex.lines ?? t.lines,
  }));
  return appendTimeline(next, {
    kind: "tool-end",
    summary: next.toolCalls[ev.toolCallId]?.fn,
    refId: ev.toolCallId,
    status: finalStatus === "err" ? "err" : "ok",
  });
};

export const onToolResult = (state: AgentViewState, ev: ToolCallResultEvent): AgentViewState =>
  updateTool(state, ev.toolCallId, (t) => ({ ...t, result: ev.content }));

// First chunk — synthesize the tool entry. toolCallName might be absent
// (some backends only set it on the first chunk that carries it); we fall
// back to "" and downstream consumers tolerate that until a later chunk
// fills it.
function initToolCall(state: AgentViewState, ev: ToolCallChunkEvent): AgentViewState {
  const id = ev.toolCallId!;
  let next: AgentViewState = {
    ...state,
    toolCalls: {
      ...state.toolCalls,
      [id]: {
        id,
        fn: ev.toolCallName ?? "",
        args: "",
        status: "running",
        duration: "LIVE",
      },
    },
  };
  if (ev.parentMessageId) {
    next = updateMessage(next, ev.parentMessageId, (m) =>
      appendBlock(m, { kind: "tool", toolCallId: id }),
    );
  }
  return next;
}

function fillToolName(state: AgentViewState, id: string, name: string): AgentViewState {
  return {
    ...state,
    toolCalls: {
      ...state.toolCalls,
      [id]: { ...state.toolCalls[id], fn: name },
    },
  };
}

export const onToolChunk = (state: AgentViewState, ev: ToolCallChunkEvent): AgentViewState => {
  if (!ev.toolCallId) return state;
  let next = state;
  const existing = next.toolCalls[ev.toolCallId];
  if (!existing) {
    next = initToolCall(next, ev);
  } else if (ev.toolCallName && !existing.fn) {
    next = fillToolName(next, ev.toolCallId, ev.toolCallName);
  }
  if (ev.delta) {
    next = updateTool(next, ev.toolCallId, (t) => ({ ...t, args: t.args + ev.delta }));
  }
  return next;
};
