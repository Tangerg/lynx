import type { AgentViewState, TimelineEntry } from "./viewState";

export type StateUpdate = (state: AgentViewState) => AgentViewState;

let timelineSeq = 0;
const TIMELINE_MAX = 500;

/** Append an idempotent, capped timeline entry to AgentViewState. */
export function appendTimelineEntry(
  entry: Omit<TimelineEntry, "id" | "ts" | "runId"> & { runId?: string | null },
): StateUpdate {
  return (state) => {
    const runId = entry.runId ?? state.run.runId;
    const key = entry.refId ?? runId;
    // Natural-key dedupe: history replay and reconnect can redeliver the same
    // run-significant event through different channels, outside stream-level
    // seenEventIds.
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
