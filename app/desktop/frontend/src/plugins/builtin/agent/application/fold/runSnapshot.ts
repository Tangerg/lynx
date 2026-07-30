import type { RunRef } from "@/rpc";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { projectRunRef } from "../view/runProjection";

export function foldRunSnapshot(state: AgentSessionView, run: RunRef): AgentSessionView {
  const projected = projectRunRef(run);
  const previous = state.runsById[run.id];
  const progress =
    previous?.status === "running" &&
    projected.status === "running" &&
    previous.activeSegmentId === projected.activeSegmentId
      ? previous.progress
      : null;
  return {
    ...state,
    runsById: {
      ...state.runsById,
      [run.id]: { ...projected, progress },
    },
  };
}
