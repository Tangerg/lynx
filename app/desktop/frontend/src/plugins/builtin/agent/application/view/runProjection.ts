import { errorDetail } from "@/rpc";
import type { ProblemData, RunMetrics, RunOutcome, RunRef, SegmentOutcome, Usage } from "@/rpc";
import type {
  AgentProblem,
  AgentRunMetrics,
  AgentRunOutcome,
  AgentRunView,
  RunUsage,
} from "@/plugins/sdk/types/agentSessionView";
import type { AgentFoldSource } from "../fold/source";

const EMPTY_USAGE: RunUsage = {
  inputTokens: 0,
  outputTokens: 0,
  cacheReadTokens: 0,
};

export function projectUsage(usage?: Usage): RunUsage {
  if (!usage) return EMPTY_USAGE;
  return {
    inputTokens: usage.inputTokens ?? 0,
    outputTokens: usage.outputTokens ?? 0,
    cacheReadTokens: usage.cacheReadTokens ?? 0,
    ...(usage.costUsd !== undefined ? { costUsd: usage.costUsd } : {}),
  };
}

export function projectRunMetrics(metrics: RunMetrics): AgentRunMetrics {
  return {
    steps: metrics.steps,
    activeDurationMillis: metrics.activeDurationMillis,
    usage: projectUsage(metrics.usage),
  };
}

export function projectProblem(problem: ProblemData): AgentProblem {
  const retryAfterSeconds = "retryAfterSeconds" in problem ? problem.retryAfterSeconds : undefined;
  return {
    message: errorDetail(problem),
    code: problem.type,
    retryAfterSeconds,
  };
}

export function projectRunOutcome(outcome: RunOutcome): AgentRunOutcome {
  switch (outcome.type) {
    case "completed":
      return { type: "completed" };
    case "timedOut":
    case "failed":
    case "lost":
      return { type: outcome.type, error: projectProblem(outcome.error) };
    case "maxSteps":
    case "maxBudget":
    case "canceled":
      return {
        type: outcome.type,
        ...(outcome.detail !== undefined ? { detail: outcome.detail } : {}),
      };
  }
}

export function projectTerminalSegmentOutcome(
  outcome: Exclude<SegmentOutcome, { type: "interrupt" | "suspended" }>,
): AgentRunOutcome {
  return projectRunOutcome(outcome);
}

export function projectRunRef(run: RunRef): AgentRunView {
  if (!run.status) {
    throw new Error(`agent.view.runProjection.statusMissing:run=${run.id}`);
  }
  if (!run.createdAt) {
    throw new Error(`agent.view.runProjection.createdAtMissing:run=${run.id}`);
  }

  const child = run.spawnedByItemId !== undefined;
  if (child && (!run.parentRunId || !run.rootRunId)) {
    throw new Error(
      `agent.view.runProjection.childLineageMissing:run=${run.id};parentRun=${run.parentRunId ?? "missing"};rootRun=${run.rootRunId ?? "missing"}`,
    );
  }
  if (!child && (run.parentRunId || run.rootRunId)) {
    throw new Error(
      `agent.view.runProjection.rootLineagePresent:run=${run.id};parentRun=${run.parentRunId ?? "missing"};rootRun=${run.rootRunId ?? "missing"}`,
    );
  }
  if (run.status === "running" && !run.activeSegmentId) {
    throw new Error(`agent.view.runProjection.activeSegmentMissing:run=${run.id}`);
  }
  if (run.status !== "running" && run.activeSegmentId) {
    throw new Error(
      `agent.view.runProjection.unexpectedActiveSegment:run=${run.id};status=${run.status};segment=${run.activeSegmentId}`,
    );
  }
  if (run.status === "finished" && (!run.outcome || !run.finishedAt)) {
    throw new Error(
      `agent.view.runProjection.terminalFactsMissing:run=${run.id};outcome=${run.outcome?.type ?? "missing"};finishedAt=${run.finishedAt ?? "missing"}`,
    );
  }
  if (run.status !== "finished" && (run.outcome || run.finishedAt)) {
    throw new Error(
      `agent.view.runProjection.unexpectedTerminalFacts:run=${run.id};status=${run.status};outcome=${run.outcome?.type ?? "missing"};finishedAt=${run.finishedAt ?? "missing"}`,
    );
  }

  return {
    id: run.id,
    sessionId: run.sessionId,
    parentRunId: child ? run.parentRunId! : null,
    rootRunId: child ? run.rootRunId! : run.id,
    spawnedByItemId: run.spawnedByItemId ?? null,
    status: run.status,
    activeSegmentId: run.activeSegmentId ?? null,
    outcome: run.outcome ? projectRunOutcome(run.outcome) : null,
    metrics: projectRunMetrics(run.metrics),
    progress: null,
    createdAt: run.createdAt,
    finishedAt: run.finishedAt ?? null,
  };
}

export function projectStartedRun(run: RunRef, source: AgentFoldSource): AgentRunView {
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
