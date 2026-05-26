// Pure (state, ...) → state helpers shared by every handler. No I/O,
// no protocol-specific shapes — just immutable updates over AgentViewState.

import type { BaseEvent } from "@ag-ui/core";
import type { CoreEventHandler } from "@/plugins/sdk";
import type { AgentViewState, ContentBlock, Message, TimelineEntry, ToolCall } from "@/protocol/agui/viewState";
import { appendTimelineEntry } from "@/plugins/sdk";

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

// Display name shown under the avatar. A real backend should drive the
// assistant name via STATE_SNAPSHOT or a session-level field — pinning
// "Sonnet 4.5" here is a placeholder until that lands.
const ROLE_DISPLAY_NAME: Record<Message["role"], string> = {
  user: "You",
  assistant: "Sonnet 4.5",
  system: "System",
};

export function nameForRole(role: Message["role"]): string {
  return ROLE_DISPLAY_NAME[role];
}

// Normalize TextMessage* role field — backends sometimes omit it or send
// "developer" / other variants. We collapse anything non-user/non-system
// to "assistant" (the common streaming case).
export function roleFromTextEvent(role: string | undefined): Message["role"] {
  if (role === "user") return "user";
  if (role === "system") return "system";
  return "assistant";
}

// Same idea for MESSAGES_SNAPSHOT — but the default lands on "system"
// (developer / unknown roles fold there) because snapshot includes both
// chat turns and protocol noise.
export function roleFromSnapshotMessage(role: string): Message["role"] {
  if (role === "user") return "user";
  if (role === "assistant") return "assistant";
  return "system";
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

// Thin sync wrapper around the SDK's `appendTimelineEntry` so core
// handlers can use the natural `(state, entry) => state` signature
// while still sharing the SDK's id-counter — important because both
// core and custom-event handlers append to the same timeline and
// owning two counters risked colliding entry ids.
export function appendTimeline(
  state: AgentViewState,
  entry: Omit<TimelineEntry, "id" | "ts" | "runId"> & { runId?: string | null },
): AgentViewState {
  return appendTimelineEntry(entry)(state);
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
        activities: { ...m.activities, [activityType]: next },
      };
    }),
  };
}
