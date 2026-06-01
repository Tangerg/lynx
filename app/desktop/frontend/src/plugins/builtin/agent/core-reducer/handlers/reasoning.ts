// Reasoning + extended-thinking handlers.
//
// Native AG-UI reasoning: REASONING_MESSAGE_START / CONTENT / END +
// fused CHUNK. Each carries an explicit messageId that scopes the
// reasoning block.
//
// Claude 3.7+ "extended thinking" phase: THINKING_TEXT_MESSAGE_* events
// have NO messageId — they're scoped by the surrounding THINKING_START/
// END pair. We translate each START into a synthetic reasoning block on
// the last assistant message; CONTENT/END operate on the most recent
// open block. THINKING_START/END themselves are no-ops — the inner
// stream lifecycle conveys "thinking happened".

import type {
  ReasoningMessageChunkEvent,
  ReasoningMessageContentEvent,
  ReasoningMessageEndEvent,
  ReasoningMessageStartEvent,
  ThinkingTextMessageContentEvent,
} from "@ag-ui/core";
import type { AgentViewState } from "@/protocol/agui/viewState";
import {
  appendBlock,
  appendTimeline,
  findActiveThinkingId,
  findLastAssistantMessageId,
  mapReasoning,
  nextThinkingId,
  updateMessage,
} from "../helpers";

export const onReasoningStart = (
  state: AgentViewState,
  ev: ReasoningMessageStartEvent,
): AgentViewState => {
  const parentId = (ev as ReasoningMessageStartEvent & { parentMessageId?: string })
    .parentMessageId;
  const targetId = parentId ?? findLastAssistantMessageId(state);
  if (!targetId) return state;
  const next = updateMessage(state, targetId, (m) =>
    appendBlock(m, { kind: "reasoning", reasoningId: ev.messageId, text: "", status: "running" }),
  );
  return appendTimeline(next, { kind: "reasoning-start", refId: ev.messageId });
};

export const onReasoningContent = (
  state: AgentViewState,
  ev: ReasoningMessageContentEvent,
): AgentViewState => mapReasoning(state, ev.messageId, (b) => ({ ...b, text: b.text + ev.delta }));

export const onReasoningEnd = (
  state: AgentViewState,
  ev: ReasoningMessageEndEvent,
): AgentViewState => {
  const next = mapReasoning(state, ev.messageId, (b) => ({ ...b, status: "complete" }));
  return appendTimeline(next, { kind: "reasoning-end", refId: ev.messageId });
};

export const onReasoningChunk = (
  state: AgentViewState,
  ev: ReasoningMessageChunkEvent,
): AgentViewState => {
  if (!ev.messageId) return state;
  const exists = state.messages.some((m) =>
    m.blocks.some((b) => b.kind === "reasoning" && b.reasoningId === ev.messageId),
  );
  let next = state;
  if (!exists) {
    const parentId =
      (ev as ReasoningMessageChunkEvent & { parentMessageId?: string }).parentMessageId ??
      findLastAssistantMessageId(next);
    if (!parentId) return state;
    next = updateMessage(next, parentId, (m) =>
      appendBlock(m, {
        kind: "reasoning",
        reasoningId: ev.messageId!,
        text: "",
        status: "running",
      }),
    );
  }
  if (ev.delta) {
    next = mapReasoning(next, ev.messageId, (b) => ({ ...b, text: b.text + ev.delta }));
  }
  return next;
};

// ---- extended-thinking (Claude 3.7+) ------------------------------------

export const onThinkingTextStart = (state: AgentViewState): AgentViewState => {
  const parentId = findLastAssistantMessageId(state);
  if (!parentId) return state;
  return updateMessage(state, parentId, (m) =>
    appendBlock(m, {
      kind: "reasoning",
      reasoningId: nextThinkingId(),
      text: "",
      status: "running",
    }),
  );
};

export const onThinkingTextContent = (
  state: AgentViewState,
  ev: ThinkingTextMessageContentEvent,
): AgentViewState => {
  const id = findActiveThinkingId(state);
  if (!id) return state;
  return mapReasoning(state, id, (b) => ({ ...b, text: b.text + ev.delta }));
};

export const onThinkingTextEnd = (state: AgentViewState): AgentViewState => {
  const id = findActiveThinkingId(state);
  if (!id) return state;
  return mapReasoning(state, id, (b) => ({ ...b, status: "complete" }));
};
