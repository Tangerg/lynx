// Built-in plugin: AG-UI protocol semantics.
//
// Every RUN_*, TEXT_MESSAGE_*, TOOL_CALL_*, REASONING_* case used to live
// inside `protocol/agui/reducer.ts`. Pulling them into a plugin means even
// the protocol layer is a (replaceable) extension — a power user can swap
// this for a custom dialect by registering a different `core-reducer` plugin
// that takes priority. The kernel reducer is now pure dispatch.

import {
  EventType,
  type ReasoningMessageContentEvent,
  type ReasoningMessageEndEvent,
  type ReasoningMessageStartEvent,
  type RunStartedEvent,
  type StepStartedEvent,
  type TextMessageContentEvent,
  type TextMessageEndEvent,
  type TextMessageStartEvent,
  type ToolCallArgsEvent,
  type ToolCallEndEvent,
  type ToolCallResultEvent,
  type ToolCallStartEvent,
} from "@ag-ui/core";
import { definePlugin } from "@/plugins/sdk";
import type {
  AgentViewState,
  ContentBlock,
  Message,
  ToolCall,
} from "@/protocol/agui/viewState";

// ---------------------------------------------------------------------------
// Helpers (copied from the old reducer — they're pure state ops, no I/O).
// ---------------------------------------------------------------------------

function nowTime(): string {
  const d = new Date();
  const h = d.getHours() % 12 || 12;
  const m = String(d.getMinutes()).padStart(2, "0");
  const meridiem = d.getHours() >= 12 ? "PM" : "AM";
  return `${h}:${m} ${meridiem}`;
}

function nameForRole(role: Message["role"]): string {
  if (role === "user") return "You";
  if (role === "assistant") return "Sonnet 4.5";
  return "System";
}

function updateMessage(
  state: AgentViewState,
  id: string,
  fn: (m: Message) => Message,
): AgentViewState {
  return {
    ...state,
    messages: state.messages.map((m) => (m.id === id ? fn(m) : m)),
  };
}

function updateTool(
  state: AgentViewState,
  id: string,
  fn: (t: ToolCall) => ToolCall,
): AgentViewState {
  const existing = state.toolCalls[id];
  if (!existing) return state;
  return { ...state, toolCalls: { ...state.toolCalls, [id]: fn(existing) } };
}

function appendBlock(m: Message, block: ContentBlock): Message {
  return { ...m, blocks: [...m.blocks, block] };
}

function appendTextDelta(m: Message, delta: string): Message {
  const blocks = m.blocks.slice();
  const last = blocks[blocks.length - 1];
  if (last && last.kind === "text" && last.streaming) {
    blocks[blocks.length - 1] = { ...last, text: last.text + delta };
    return { ...m, blocks };
  }
  blocks.push({ kind: "text", text: delta, streaming: true });
  return { ...m, blocks };
}

function mapReasoning(
  state: AgentViewState,
  reasoningId: string,
  fn: (b: Extract<ContentBlock, { kind: "reasoning" }>) => ContentBlock,
): AgentViewState {
  return {
    ...state,
    messages: state.messages.map((m) => {
      let touched = false;
      const blocks = m.blocks.map((b) => {
        if (b.kind !== "reasoning" || b.reasoningId !== reasoningId) return b;
        touched = true;
        return fn(b);
      });
      return touched ? { ...m, blocks } : m;
    }),
  };
}

function findLastAssistantMessageId(state: AgentViewState): string | null {
  for (let i = state.messages.length - 1; i >= 0; i--) {
    if (state.messages[i].role === "assistant") return state.messages[i].id;
  }
  return null;
}

// ---------------------------------------------------------------------------
// Per-event handlers — pure (state, event) → state.
// ---------------------------------------------------------------------------

const onRunStarted = (state: AgentViewState, ev: RunStartedEvent): AgentViewState => ({
  ...state,
  run: { ...state.run, running: true, threadId: ev.threadId, runId: ev.runId },
});

const onRunFinished = (state: AgentViewState): AgentViewState => ({
  ...state,
  run: { ...state.run, running: false },
});

const onStepStarted = (state: AgentViewState, ev: StepStartedEvent): AgentViewState => ({
  ...state,
  run: { ...state.run, activity: ev.stepName },
});

const onTextStart = (state: AgentViewState, ev: TextMessageStartEvent): AgentViewState => {
  const role: Message["role"] = ev.role === "user" ? "user" : ev.role === "system" ? "system" : "assistant";
  const msg: Message = {
    id: ev.messageId,
    role,
    who: nameForRole(role),
    time: nowTime(),
    blocks: [],
  };
  return { ...state, messages: [...state.messages, msg] };
};

const onTextContent = (state: AgentViewState, ev: TextMessageContentEvent): AgentViewState =>
  updateMessage(state, ev.messageId, (m) => appendTextDelta(m, ev.delta));

const onTextEnd = (state: AgentViewState, ev: TextMessageEndEvent): AgentViewState =>
  updateMessage(state, ev.messageId, (m) => ({
    ...m,
    blocks: m.blocks.map((b) =>
      b.kind === "text" && b.streaming ? { ...b, streaming: false } : b,
    ),
  }));

const onToolStart = (state: AgentViewState, ev: ToolCallStartEvent): AgentViewState => {
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
  return next;
};

const onToolArgs = (state: AgentViewState, ev: ToolCallArgsEvent): AgentViewState =>
  updateTool(state, ev.toolCallId, (t) => ({ ...t, args: t.args + ev.delta }));

const onToolEnd = (state: AgentViewState, ev: ToolCallEndEvent): AgentViewState => {
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
  return updateTool(state, ev.toolCallId, (t) => ({
    ...t,
    status: ex.status ?? "ok",
    duration: ex.durationMs != null ? `${ex.durationMs}ms` : t.duration === "LIVE" ? "—" : t.duration,
    added: ex.added ?? t.added,
    removed: ex.removed ?? t.removed,
    hits: ex.hits ?? t.hits,
    lines: ex.lines ?? t.lines,
  }));
};

const onToolResult = (state: AgentViewState, ev: ToolCallResultEvent): AgentViewState =>
  updateTool(state, ev.toolCallId, (t) => ({ ...t, result: ev.content }));

const onReasoningStart = (state: AgentViewState, ev: ReasoningMessageStartEvent): AgentViewState => {
  const parentId = (ev as ReasoningMessageStartEvent & { parentMessageId?: string }).parentMessageId;
  const targetId = parentId ?? findLastAssistantMessageId(state);
  if (!targetId) return state;
  return updateMessage(state, targetId, (m) =>
    appendBlock(m, { kind: "reasoning", reasoningId: ev.messageId, text: "", streaming: true }),
  );
};

const onReasoningContent = (state: AgentViewState, ev: ReasoningMessageContentEvent): AgentViewState =>
  mapReasoning(state, ev.messageId, (b) => ({ ...b, text: b.text + ev.delta }));

const onReasoningEnd = (state: AgentViewState, ev: ReasoningMessageEndEvent): AgentViewState =>
  mapReasoning(state, ev.messageId, (b) => ({ ...b, streaming: false }));

// ---------------------------------------------------------------------------
// Plugin
// ---------------------------------------------------------------------------

export default definePlugin({
  name: "lyra.builtin.core-reducer",
  version: "1.0.0",
  setup({ host }) {
    // Run lifecycle.
    host.agui.onCore(EventType.RUN_STARTED, (s, ev) => onRunStarted(s, ev as RunStartedEvent));
    host.agui.onCore(EventType.RUN_FINISHED, onRunFinished);
    host.agui.onCore(EventType.RUN_ERROR, onRunFinished);
    host.agui.onCore(EventType.STEP_STARTED, (s, ev) => onStepStarted(s, ev as StepStartedEvent));

    // Text messages.
    host.agui.onCore(EventType.TEXT_MESSAGE_START,   (s, ev) => onTextStart  (s, ev as TextMessageStartEvent));
    host.agui.onCore(EventType.TEXT_MESSAGE_CONTENT, (s, ev) => onTextContent(s, ev as TextMessageContentEvent));
    host.agui.onCore(EventType.TEXT_MESSAGE_END,     (s, ev) => onTextEnd    (s, ev as TextMessageEndEvent));

    // Tool calls.
    host.agui.onCore(EventType.TOOL_CALL_START,  (s, ev) => onToolStart (s, ev as ToolCallStartEvent));
    host.agui.onCore(EventType.TOOL_CALL_ARGS,   (s, ev) => onToolArgs  (s, ev as ToolCallArgsEvent));
    host.agui.onCore(EventType.TOOL_CALL_END,    (s, ev) => onToolEnd   (s, ev as ToolCallEndEvent));
    host.agui.onCore(EventType.TOOL_CALL_RESULT, (s, ev) => onToolResult(s, ev as ToolCallResultEvent));

    // Reasoning. REASONING_START / REASONING_END are span markers we don't
    // need to act on (we open/close via REASONING_MESSAGE_START / END).
    host.agui.onCore(EventType.REASONING_MESSAGE_START,   (s, ev) => onReasoningStart  (s, ev as ReasoningMessageStartEvent));
    host.agui.onCore(EventType.REASONING_MESSAGE_CONTENT, (s, ev) => onReasoningContent(s, ev as ReasoningMessageContentEvent));
    host.agui.onCore(EventType.REASONING_MESSAGE_END,     (s, ev) => onReasoningEnd    (s, ev as ReasoningMessageEndEvent));
  },
});
