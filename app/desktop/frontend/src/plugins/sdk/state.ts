// State update helpers for AG-UI CUSTOM event handlers.
//
// Handlers return a `StateUpdate` (state → state). Rather than make plugin
// authors touch the AgentViewState shape directly, they compose updates from
// these helpers:
//
//   host.agui.on("monitoring.cpu", (value) =>
//     appendBlockToLatestAssistant({ kind: "cpuChart", series: value.series })
//   );

import type { StateUpdate } from "./types";
import type {
  AgentViewState,
  ContentBlock,
  ContentBlockKind,
  ContentBlockMap,
  Message,
  PlanItem,
} from "@/protocol/agui/viewState";

/** Append a content block to a specific message by id. No-op if not found. */
export function appendBlockToMessage<K extends ContentBlockKind>(
  messageId: string,
  block: ContentBlockMap[K],
): StateUpdate {
  return (state) =>
    updateMessage(state, messageId, (m) => ({
      ...m,
      blocks: [...m.blocks, block as ContentBlock],
    }));
}

/** Append a content block to the most recent assistant message. No-op if none. */
export function appendBlockToLatestAssistant<K extends ContentBlockKind>(
  block: ContentBlockMap[K],
): StateUpdate {
  return (state) => {
    const targetId = findLastAssistantId(state);
    if (!targetId) return state;
    return updateMessage(state, targetId, (m) => ({
      ...m,
      blocks: [...m.blocks, block as ContentBlock],
    }));
  };
}

/** Replace the run plan wholesale. */
export function setPlan(items: PlanItem[]): StateUpdate {
  return (state) => ({ ...state, plan: items });
}

/** Patch one or more run-state fields. */
export function patchRun(patch: Partial<AgentViewState["run"]>): StateUpdate {
  return (state) => ({ ...state, run: { ...state.run, ...patch } });
}

/** Compose a sequence of updates. Useful when one handler does several things. */
export function compose(...updates: StateUpdate[]): StateUpdate {
  return (state) => updates.reduce((acc, u) => u(acc), state);
}

// ---- internal -------------------------------------------------------------

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

function findLastAssistantId(state: AgentViewState): string | null {
  for (let i = state.messages.length - 1; i >= 0; i--) {
    if (state.messages[i].role === "assistant") return state.messages[i].id;
  }
  return null;
}
