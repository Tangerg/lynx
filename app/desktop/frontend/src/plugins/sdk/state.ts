// State update helpers for `custom` StreamEvent handlers.
//
// Handlers return a `StateUpdate` (state → state). Rather than make plugin
// authors touch the AgentViewState shape directly, they compose updates from
// these helpers:
//
//   host.events.onCustom("monitoring.cpu", (value) =>
//     appendBlockToLatestAssistant({ kind: "cpuChart", series: value.series })
//   );

import type { StateUpdate } from "./types";
import type {
  AgentViewState,
  Message,
  PlanItem,
  TimelineEntry,
} from "@/plugins/sdk/types/agentView";
import type {
  ContentBlock,
  ContentBlockKind,
  ContentBlockMap,
} from "@/plugins/sdk/types/contentBlock";
import { appendTimelineEntry as appendProtocolTimelineEntry } from "@/plugins/sdk/types/agentTimeline";

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

/**
 * Patch every content block matching `predicate`, across all messages.
 * HITL result handlers use this to settle a block by its requestId when the
 * result event doesn't carry the parent message id (so a by-id lookup isn't
 * possible). `predicate` is a type guard so `patch` receives the narrowed
 * block type. Messages with no match keep their identity (no needless clone).
 */
export function patchBlocksWhere<B extends ContentBlock>(
  predicate: (block: ContentBlock) => block is B,
  patch: (block: B) => B,
): StateUpdate {
  return (state) => ({
    ...state,
    messages: state.messages.map((m) =>
      m.blocks.some(predicate)
        ? { ...m, blocks: m.blocks.map((b) => (predicate(b) ? patch(b) : b)) }
        : m,
    ),
  });
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

/** Append a structured entry to the run timeline. Custom-event handlers
 *  use this to surface approval / checkpoint / other domain markers in
 *  the Run Timeline view. */
export function appendTimelineEntry(
  entry: Omit<TimelineEntry, "id" | "ts" | "runId"> & { runId?: string | null },
): StateUpdate {
  return appendProtocolTimelineEntry(entry);
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

function findLastAssistantId(state: AgentViewState): string | null {
  return state.messages.findLast((m) => m.role === "assistant")?.id ?? null;
}
