import type { AgentRunFact, AgentSegmentOutcome } from "@/plugins/sdk";
import type {
  AgentRunMetrics,
  AgentRunOutcome,
  AgentRunProgress,
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
  projectRunMetrics,
  projectStartedRun,
  projectTerminalSegmentOutcome,
} from "../view/runProjection";
import { isAgentRunFailure } from "../view/runOutcome";

function sameRunMetrics(left: AgentRunMetrics, right: AgentRunMetrics): boolean {
  return (
    left.steps === right.steps &&
    left.activeDurationMillis === right.activeDurationMillis &&
    left.usage.inputTokens === right.usage.inputTokens &&
    left.usage.outputTokens === right.usage.outputTokens &&
    left.usage.cacheReadTokens === right.usage.cacheReadTokens &&
    left.usage.costUsd === right.usage.costUsd
  );
}

function sameRunOutcome(left: AgentRunOutcome | null, right: AgentRunOutcome): boolean {
  if (!left || left.type !== right.type) return false;
  if (left.type === "completed") return true;
  if (isAgentRunFailure(left) && isAgentRunFailure(right)) {
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
  outcome: AgentSegmentOutcome,
  metrics: AgentRunMetrics,
  source: AgentFoldSource,
): boolean {
  const run = state.runsById[source.runId];
  if (!run) return false;
  const projectedMetrics = projectRunMetrics(metrics);
  if (!sameRunMetrics(run.metrics, projectedMetrics)) return false;
  if (outcome.type === "interrupt") {
    if (run.status !== "waiting") return false;
    const rootRunId = run.rootRunId;
    const open = new Set(
      state.pendingInterrupts
        .filter((group) => group.rootRunId === rootRunId)
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
  run: AgentRunFact,
  source: AgentFoldSource,
): AgentSessionView {
  const started = projectStartedRun(run, source);
  const previous = state.runsById[run.id];
  const exactReplay = state.timeline.some(
    (entry) => entry.id === `timeline:${source.eventId}:run-start`,
  );
  if (previous) {
    if (previous.status === "finished") {
      if (exactReplay) return state;
      throw new Error(
        `agent.fold.runStatusMismatch:event=segment.started;run=${run.id};status=finished;expected=waitingOrAbsent`,
      );
    }
    if (previous.status === "running") {
      if (previous.activeSegmentId === source.segmentId) return state;
      throw new Error(
        `agent.fold.segmentMismatch:event=segment.started;run=${run.id};eventSegment=${source.segmentId ?? "missing"};activeSegment=${previous.activeSegmentId ?? "missing"}`,
      );
    }
    if (exactReplay) return state;
  }
  const next: AgentSessionView = {
    ...state,
    commandError: null,
    runsById: {
      ...state.runsById,
      [run.id]: started,
    },
  };
  return appendTimelineEntry(timelineEntry(source, "run-start"))(next);
}

export function onRunProgress(
  state: AgentSessionView,
  progress: AgentRunProgress,
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
        ...(progress.usage ? { usage: { ...progress.usage } } : {}),
        ...(progress.contextTokens !== undefined ? { contextTokens: progress.contextTokens } : {}),
      },
    };
  });
}

export function onRunFinished(
  state: AgentSessionView,
  outcome: AgentSegmentOutcome,
  metrics: AgentRunMetrics,
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
        progress: settledContextProgress(run.progress),
      };
    }
    return {
      ...run,
      status: "finished",
      activeSegmentId: null,
      outcome: projectTerminalSegmentOutcome(outcome),
      metrics: projectRunMetrics(metrics),
      progress: settledContextProgress(run.progress),
      finishedAt: source.timestamp,
    };
  });

  if (outcome.type === "suspended") return next;
  if (outcome.type === "interrupt") {
    const rootRunId = next.runsById[source.runId]!.rootRunId;
    const byRunId = new Map<string, PendingInterrupt[]>();
    for (const interrupt of outcome.interrupts) {
      const runId = interrupt.runId;
      const pending = byRunId.get(runId) ?? [];
      pending.push({ itemId: interrupt.itemId, kind: interrupt.type });
      byRunId.set(runId, pending);
    }
    for (const [runId, interrupts] of byRunId) {
      next = mergePendingInterrupts(next, runId, rootRunId, interrupts);
    }
    for (const interrupt of outcome.interrupts) {
      const runId = interrupt.runId;
      next = materializeInterrupt(next, interrupt, { ...source, runId }, rootRunId);
    }
    return next;
  }

  next = settleRunPendingInterrupts(next, source.runId);
  const projectedOutcome = projectTerminalSegmentOutcome(outcome);
  if (isAgentRunFailure(projectedOutcome)) {
    const problem = projectedOutcome.error;
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
      summary: terminalOutcomeSummary(projectedOutcome),
    }),
  )(next);
}

/** Activity, step and provisional usage expire at the segment boundary. The
 * latest prompt footprint does not: it remains the Session's context-window
 * reading until a successor Run publishes its own value. */
function settledContextProgress(
  progress: AgentRunProgress | null,
): Pick<AgentRunProgress, "contextTokens"> | null {
  return progress?.contextTokens === undefined ? null : { contextTokens: progress.contextTokens };
}

function terminalOutcomeSummary(outcome: AgentRunOutcome): string | undefined {
  if (outcome.type === "completed") return undefined;
  if (isAgentRunFailure(outcome))
    return outcome.error.message ?? outcome.error.code ?? outcome.type;
  return outcome.detail ?? outcome.type;
}

function mergePendingInterrupts(
  state: AgentSessionView,
  runId: string,
  rootRunId: string,
  interrupts: PendingInterrupt[],
): AgentSessionView {
  if (interrupts.length === 0) return state;
  const run = state.runsById[runId];
  if (!run) {
    throw new Error(`agent.fold.runMissing:event=segment.finished.interrupt;run=${runId}`);
  }
  const existingGroup = state.pendingInterrupts.find(
    (group) => group.runId === runId && group.rootRunId === rootRunId,
  );
  const existingIds = new Set(existingGroup?.interrupts.map((interrupt) => interrupt.itemId));
  const fresh = interrupts.filter((interrupt) => !existingIds.has(interrupt.itemId));
  if (fresh.length === 0) return state;
  if (!existingGroup) {
    return {
      ...state,
      pendingInterrupts: [
        ...state.pendingInterrupts,
        { runId, rootRunId, sessionId: run.sessionId, interrupts: fresh },
      ],
    };
  }
  return {
    ...state,
    pendingInterrupts: state.pendingInterrupts.map((group) =>
      group.runId === runId && group.rootRunId === rootRunId
        ? { ...group, interrupts: [...group.interrupts, ...fresh] }
        : group,
    ),
  };
}
