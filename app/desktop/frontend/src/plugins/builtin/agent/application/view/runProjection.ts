import type { AgentRunFact, AgentSegmentOutcome } from "@/plugins/sdk";
import type {
  AgentRunMetrics,
  AgentRunOutcome,
  AgentRunView,
} from "@/plugins/sdk/types/agentSessionView";
import type { AgentFoldSource } from "../fold/source";

export function projectRunMetrics(metrics: AgentRunMetrics): AgentRunMetrics {
  return {
    steps: metrics.steps,
    activeDurationMillis: metrics.activeDurationMillis,
    usage: { ...metrics.usage },
  };
}

export function projectTerminalSegmentOutcome(
  outcome: Exclude<AgentSegmentOutcome, { type: "interrupt" | "suspended" }>,
): AgentRunOutcome {
  return outcome;
}

export function projectRunRef(run: AgentRunFact): AgentRunView {
  return {
    id: run.id,
    sessionId: run.sessionId,
    parentRunId: run.parentRunId,
    rootRunId: run.rootRunId,
    spawnedByItemId: run.spawnedByItemId,
    status: run.status,
    activeSegmentId: run.activeSegmentId,
    outcome: run.outcome,
    modelSelection: run.modelSelection ? { ...run.modelSelection } : null,
    metrics: projectRunMetrics(run.metrics),
    progress: run.contextTokens === undefined ? null : { contextTokens: run.contextTokens },
    createdAt: run.createdAt,
    finishedAt: run.finishedAt,
  };
}

export function projectStartedRun(run: AgentRunFact, source: AgentFoldSource): AgentRunView {
  if (run.id !== source.runId) {
    throw new Error(`agent.fold.startedRunMismatch:eventRun=${source.runId};payloadRun=${run.id}`);
  }
  if (!source.segmentId) {
    throw new Error(`agent.fold.startedSegmentMissing:run=${run.id}`);
  }
  const projected = projectRunRef(run);
  if (projected.status !== "running") {
    throw new Error(
      `agent.fold.startedStatusMismatch:run=${run.id};status=${projected.status};expected=running`,
    );
  }
  if (projected.activeSegmentId !== source.segmentId) {
    throw new Error(
      `agent.fold.startedSegmentMismatch:run=${run.id};payloadSegment=${projected.activeSegmentId ?? "missing"};eventSegment=${source.segmentId}`,
    );
  }
  return projected;
}
