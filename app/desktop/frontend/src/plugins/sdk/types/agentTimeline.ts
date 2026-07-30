import type { AgentSessionView, TimelineEntry } from "@/plugins/sdk/types/agentSessionView";

export type StateUpdate = (state: AgentSessionView) => AgentSessionView;

const TIMELINE_MAX = 500;

/** Append an idempotent, capped timeline entry to an AgentSessionView. */
export function appendTimelineEntry(entry: TimelineEntry): StateUpdate {
  return (state) => {
    if (state.timeline.some((existing) => existing.id === entry.id)) return state;
    const next = [...state.timeline, entry];
    return {
      ...state,
      timeline: next.length > TIMELINE_MAX ? next.slice(next.length - TIMELINE_MAX) : next,
    };
  };
}
