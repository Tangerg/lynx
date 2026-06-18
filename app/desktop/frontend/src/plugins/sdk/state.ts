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
  ContentBlock,
  ContentBlockKind,
  ContentBlockMap,
  Message,
  PlanItem,
  TimelineEntry,
} from "@/protocol/run/viewState";

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
 *  the Run Timeline view. Core handlers append directly via helpers; this
 *  SDK helper exists so plugins outside core-reducer can do the same. */
let timelineSeq = 0;
// Long-session cap. Every RUN_*/STEP_*/TOOL_*/REASONING_* fold pushes
// an entry, so a multi-hour session can pile up thousands of entries —
// timeline view renders fine but the AgentViewState clone cost on every
// reduce + every render scales linearly with this array. Newest 500 is
// enough to drive the audit panel + run digest; older entries drop FIFO.
const TIMELINE_MAX = 500;
export function appendTimelineEntry(
  entry: Omit<TimelineEntry, "id" | "ts" | "runId"> & { runId?: string | null },
): StateUpdate {
  return (state) => {
    const runId = entry.runId ?? state.run.runId;
    // Idempotent on the natural key (kind + refId, or kind + runId for run-level
    // entries): the same run-significant event can arrive twice — items.list
    // history replay and a runs.subscribe reconnect/reload replay are separate
    // delivery channels, so the stream's seenEventIds can't dedupe across them.
    // Skip the re-delivery rather than double the Timeline row (+ inflate runDigest).
    const key = entry.refId ?? runId;
    if (key && state.timeline.some((e) => e.kind === entry.kind && (e.refId ?? e.runId) === key)) {
      return state;
    }
    timelineSeq += 1;
    const full: TimelineEntry = {
      id: `tl:${Date.now()}:${timelineSeq}`,
      ts: Date.now(),
      runId,
      kind: entry.kind,
      summary: entry.summary,
      refId: entry.refId,
      status: entry.status,
    };
    const next = [...state.timeline, full];
    return {
      ...state,
      timeline: next.length > TIMELINE_MAX ? next.slice(next.length - TIMELINE_MAX) : next,
    };
  };
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
