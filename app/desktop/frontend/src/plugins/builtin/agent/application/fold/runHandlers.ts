import type { RunMetrics, RunProgress, RunRef, SegmentOutcome } from "@/rpc";
import type {
  AgentRunMetrics,
  AgentRunOutcome,
  AgentRunView,
  AgentSessionView,
  PendingInterrupt,
  TimelineEntry,
} from "@/plugins/sdk/types/agentSessionView";
import { appendTimelineEntry } from "@/plugins/sdk";
import { settleRunPendingInterrupts } from "./fold";
import { materializeInterrupt } from "./interruptMaterialization";
import type { AgentFoldSource } from "./source";
import { sourceTimestamp } from "./source";
import {
  projectProblem,
  projectRunMetrics,
  projectStartedRun,
  projectTerminalSegmentOutcome,
  projectUsage,
} from "../view/runProjection";

function sameRunMetrics(left: AgentRunMetrics, right: AgentRunMetrics): boolean {
  return (
    left.steps === right.steps &&
    left.activeDurationMs === right.activeDurationMs &&
    left.usage.inputTokens === right.usage.inputTokens &&
    left.usage.outputTokens === right.usage.outputTokens &&
    left.usage.cacheReadTokens === right.usage.cacheReadTokens &&
    left.usage.costUsd === right.usage.costUsd
  );
}

function sameRunOutcome(left: AgentRunOutcome | null, right: AgentRunOutcome): boolean {
  if (!left || left.type !== right.type) return false;
  if (left.type === "completed") return true;
  if (left.type === "error" && right.type === "error") {
    return (
      left.error.code === right.error.code &&
      left.error.message === right.error.message &&
      left.error.retryAfterSeconds === right.error.retryAfterSeconds
    );
  }
  if (
    (left.type === "maxSteps" || left.type === "maxBudget" || left.type === "canceled") &&
    right.type === left.type
  ) {
    return left.detail === right.detail;
  }
  return false;
}

function isDuplicateRunFinish(
  state: AgentSessionView,
  outcome: SegmentOutcome,
  metrics: RunMetrics,
  source: AgentFoldSource,
): boolean {
  const run = state.runsById[source.runId];
  if (!run) return false;
  const projectedMetrics = projectRunMetrics(metrics);
  if (!sameRunMetrics(run.metrics, projectedMetrics)) return false;
  if (outcome.type === "interrupt") {
    if (run.status !== "waiting") return false;
    const open = new Set(
      state.pendingInterrupts
        .filter((group) => group.runId === run.id)
        .flatMap((group) => group.interrupts.map((interrupt) => interrupt.itemId)),
    );
    return outcome.interrupts.every((interrupt) => open.has(interrupt.itemId));
  }
  if (outcome.type === "suspended") return run.status === "waiting";
  return (
    run.status === "finished" &&
    run.finishedAt === source.timestamp &&
    sameRunOutcome(run.outcome, projectTerminalSegmentOutcome(outcome))
  );
}

function updateRun(
  state: AgentSessionView,
  runId: string,
  eventType: "segment.progress" | "segment.finished",
  update: (run: AgentRunView) => AgentRunView,
): AgentSessionView {
  const run = state.runsById[runId];
  if (!run) {
    throw new Error(`agent.fold.runMissing:event=${eventType};run=${runId}`);
  }
  return {
    ...state,
    runsById: { ...state.runsById, [runId]: update(run) },
  };
}

function timelineEntry(
  source: AgentFoldSource,
  kind: TimelineEntry["kind"],
  patch: Partial<Omit<TimelineEntry, "id" | "ts" | "kind" | "runId">> = {},
): TimelineEntry {
  return {
    id: `timeline:${source.eventId}:${kind}`,
    ts: sourceTimestamp(source),
    kind,
    runId: source.runId,
    ...patch,
  };
}

export function onRunStarted(
  state: AgentSessionView,
  run: RunRef,
  source: AgentFoldSource,
): AgentSessionView {
  const started = projectStartedRun(run, source);
  const previous = state.runsById[run.id];
  const sameSegment = previous?.activeSegmentId === source.segmentId;
  const next: AgentSessionView = {
    ...state,
    commandError: null,
    runsById: {
      ...state.runsById,
      [run.id]: sameSegment ? { ...started, progress: previous.progress } : started,
    },
  };
  return appendTimelineEntry(timelineEntry(source, "run-start"))(next);
}

export function onRunProgress(
  state: AgentSessionView,
  progress: RunProgress,
  source: AgentFoldSource,
): AgentSessionView {
  return updateRun(state, source.runId, "segment.progress", (run) => {
    if (run.status !== "running") {
      throw new Error(
        `agent.fold.runStatusMismatch:event=segment.progress;run=${run.id};status=${run.status};expected=running`,
      );
    }
    if (source.segmentId !== run.activeSegmentId) {
      throw new Error(
        `agent.fold.segmentMismatch:event=segment.progress;run=${run.id};eventSegment=${source.segmentId ?? "missing"};activeSegment=${run.activeSegmentId ?? "missing"}`,
      );
    }
    return {
      ...run,
      progress: {
        ...run.progress,
        ...(progress.step !== undefined ? { step: progress.step } : {}),
        ...(progress.activity !== undefined ? { activity: progress.activity } : {}),
        ...(progress.usage ? { usage: projectUsage(progress.usage) } : {}),
        ...(progress.contextTokens !== undefined ? { contextTokens: progress.contextTokens } : {}),
      },
    };
  });
}

export function onRunFinished(
  state: AgentSessionView,
  outcome: SegmentOutcome,
  metrics: RunMetrics,
  source: AgentFoldSource,
): AgentSessionView {
  if (isDuplicateRunFinish(state, outcome, metrics, source)) return state;
  let next = updateRun(state, source.runId, "segment.finished", (run) => {
    if (run.status !== "running") {
      throw new Error(
        `agent.fold.runStatusMismatch:event=segment.finished;run=${run.id};status=${run.status};expected=running`,
      );
    }
    if (source.segmentId !== run.activeSegmentId) {
      throw new Error(
        `agent.fold.segmentMismatch:event=segment.finished;run=${run.id};eventSegment=${source.segmentId ?? "missing"};activeSegment=${run.activeSegmentId ?? "missing"}`,
      );
    }
    if (outcome.type === "interrupt" || outcome.type === "suspended") {
      return {
        ...run,
        status: "waiting",
        activeSegmentId: null,
        outcome: null,
        metrics: projectRunMetrics(metrics),
        progress: null,
      };
    }
    return {
      ...run,
      status: "finished",
      activeSegmentId: null,
      outcome: projectTerminalSegmentOutcome(outcome),
      metrics: projectRunMetrics(metrics),
      progress: null,
      finishedAt: source.timestamp,
    };
  });

  if (outcome.type === "suspended") return next;
  if (outcome.type === "interrupt") {
    next = mergePendingInterrupts(
      next,
      source.runId,
      outcome.interrupts.map((interrupt) => ({
        itemId: interrupt.itemId,
        kind: interrupt.type,
      })),
    );
    for (const interrupt of outcome.interrupts) {
      next = materializeInterrupt(next, interrupt, source);
    }
    return next;
  }

  next = settleRunPendingInterrupts(next, source.runId);
  if (outcome.type === "error") {
    const problem = projectProblem(outcome.error);
    return appendTimelineEntry(
      timelineEntry(source, "run-error", {
        status: "err",
        summary: problem.message ?? problem.code,
      }),
    )(next);
  }

  return appendTimelineEntry(
    timelineEntry(source, "run-end", {
      status: outcome.type === "completed" ? "ok" : undefined,
      summary: outcome.type === "completed" ? undefined : (outcome.detail ?? outcome.type),
    }),
  )(next);
}

function mergePendingInterrupts(
  state: AgentSessionView,
  runId: string,
  interrupts: PendingInterrupt[],
): AgentSessionView {
  if (interrupts.length === 0) return state;
  const run = state.runsById[runId]!;
  const existingGroup = state.pendingInterrupts.find((group) => group.runId === runId);
  const existingIds = new Set(existingGroup?.interrupts.map((interrupt) => interrupt.itemId));
  const fresh = interrupts.filter((interrupt) => !existingIds.has(interrupt.itemId));
  if (fresh.length === 0) return state;
  if (!existingGroup) {
    return {
      ...state,
      pendingInterrupts: [
        ...state.pendingInterrupts,
        { runId, sessionId: run.sessionId, interrupts: fresh },
      ],
    };
  }
  return {
    ...state,
    pendingInterrupts: state.pendingInterrupts.map((group) =>
      group.runId === runId ? { ...group, interrupts: [...group.interrupts, ...fresh] } : group,
    ),
  };
}
