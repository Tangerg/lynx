// Pure (state, ...) → state helpers shared by every handler. No I/O,
// no protocol-specific shapes — just immutable updates over AgentViewState.

import type { BaseEvent } from "@ag-ui/core";
import type { CoreEventHandler } from "@/plugins/sdk";
import type { AgentViewState, ContentBlock, Message, ToolCall } from "@/protocol/agui/viewState";

// Erases each handler's specific event variant down to BaseEvent so a
// uniform `[EventType, CoreEventHandler]` table can carry them all. The
// per-handler signature still type-checks the event payload it
// destructures; the cast is only for the table's homogeneous shape.
export function bind<E extends BaseEvent>(
  fn: (state: AgentViewState, ev: E) => AgentViewState,
): CoreEventHandler {
  return fn as CoreEventHandler;
}

export function nowTime(): string {
  const d = new Date();
  const h = d.getHours() % 12 || 12;
  const m = String(d.getMinutes()).padStart(2, "0");
  const meridiem = d.getHours() >= 12 ? "PM" : "AM";
  return `${h}:${m} ${meridiem}`;
}

export function nameForRole(role: Message["role"]): string {
  if (role === "user") return "You";
  if (role === "assistant") return "Sonnet 4.5";
  return "System";
}

export function updateMessage(
  state: AgentViewState,
  id: string,
  fn: (m: Message) => Message,
): AgentViewState {
  return {
    ...state,
    messages: state.messages.map((m) => (m.id === id ? fn(m) : m)),
  };
}

export function updateTool(
  state: AgentViewState,
  id: string,
  fn: (t: ToolCall) => ToolCall,
): AgentViewState {
  const existing = state.toolCalls[id];
  if (!existing) return state;
  return { ...state, toolCalls: { ...state.toolCalls, [id]: fn(existing) } };
}

export function appendBlock(m: Message, block: ContentBlock): Message {
  return { ...m, blocks: [...m.blocks, block] };
}

export function appendTextDelta(m: Message, delta: string): Message {
  const blocks = m.blocks.slice();
  const last = blocks[blocks.length - 1];
  if (last && last.kind === "text" && last.streaming) {
    blocks[blocks.length - 1] = { ...last, text: last.text + delta };
    return { ...m, blocks };
  }
  blocks.push({ kind: "text", text: delta, streaming: true });
  return { ...m, blocks };
}

export function mapReasoning(
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

export function findLastAssistantMessageId(state: AgentViewState): string | null {
  for (let i = state.messages.length - 1; i >= 0; i--) {
    if (state.messages[i].role === "assistant") return state.messages[i].id;
  }
  return null;
}

export function findMessageById(state: AgentViewState, id: string): Message | undefined {
  return state.messages.find((m) => m.id === id);
}

// Walk messages backwards for the most recent still-streaming reasoning
// block. That's the "currently open" thinking block we should write to.
export function findActiveThinkingId(state: AgentViewState): string | null {
  for (let i = state.messages.length - 1; i >= 0; i--) {
    const m = state.messages[i];
    for (let j = m.blocks.length - 1; j >= 0; j--) {
      const b = m.blocks[j];
      if (b.kind === "reasoning" && b.streaming) return b.reasoningId;
    }
  }
  return null;
}

let thinkingBlockSeq = 0;
export function nextThinkingId(): string {
  thinkingBlockSeq += 1;
  return `thinking:${Date.now()}:${thinkingBlockSeq}`;
}

export function updateActivity(
  state: AgentViewState,
  messageId: string,
  activityType: string,
  fn: (prev: unknown) => unknown,
): AgentViewState {
  return {
    ...state,
    messages: state.messages.map((m) => {
      if (m.id !== messageId) return m;
      const prev = m.activities?.[activityType];
      const next = fn(prev);
      return {
        ...m,
        activities: { ...(m.activities ?? {}), [activityType]: next },
      };
    }),
  };
}
