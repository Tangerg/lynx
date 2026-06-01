// Text-message stream handlers — TEXT_MESSAGE_START / CONTENT / END,
// plus the fused TEXT_MESSAGE_CHUNK variant that synthesizes the
// message on first chunk.

import type {
  TextMessageChunkEvent,
  TextMessageContentEvent,
  TextMessageEndEvent,
  TextMessageStartEvent,
} from "@ag-ui/core";
import type { AgentViewState, Message } from "@/protocol/agui/viewState";
import {
  appendTextDelta,
  findMessageById,
  nameForRole,
  nowTime,
  roleFromTextEvent,
  updateMessage,
} from "../helpers";

export const onTextStart = (state: AgentViewState, ev: TextMessageStartEvent): AgentViewState => {
  // Defensive dedup: if a TEXT_MESSAGE_START fires twice for the same
  // messageId (HMR replay, agent retry, mock-server quirk), pushing
  // unconditionally produces two messages with identical ids — React
  // then logs "Encountered two children with the same key" on every
  // subsequent render and the DEV warning loop tanks the frame rate.
  if (findMessageById(state, ev.messageId)) return state;
  const role = roleFromTextEvent(ev.role);
  const msg: Message = {
    id: ev.messageId,
    role,
    who: nameForRole(role),
    time: nowTime(),
    blocks: [],
  };
  return { ...state, messages: [...state.messages, msg] };
};

export const onTextContent = (state: AgentViewState, ev: TextMessageContentEvent): AgentViewState =>
  updateMessage(state, ev.messageId, (m) => appendTextDelta(m, ev.delta));

export const onTextEnd = (state: AgentViewState, ev: TextMessageEndEvent): AgentViewState =>
  updateMessage(state, ev.messageId, (m) => ({
    ...m,
    blocks: m.blocks.map((b) =>
      b.kind === "text" && b.status === "running" ? { ...b, status: "complete" } : b,
    ),
  }));

// First chunk for this messageId materializes the message; later chunks
// just append. No explicit END — closure rides on RUN_FINISHED or a
// later non-chunk END event.
export const onTextChunk = (state: AgentViewState, ev: TextMessageChunkEvent): AgentViewState => {
  if (!ev.messageId) return state;
  let next = state;
  if (!findMessageById(next, ev.messageId)) {
    const role = roleFromTextEvent(ev.role);
    next = {
      ...next,
      messages: [
        ...next.messages,
        { id: ev.messageId, role, who: nameForRole(role), time: nowTime(), blocks: [] },
      ],
    };
  }
  if (ev.delta) {
    next = updateMessage(next, ev.messageId, (m) => appendTextDelta(m, ev.delta!));
  }
  return next;
};
