import type { RunMetrics, RunProgress, RunRef, SegmentOutcome, Usage } from "@/rpc";
import type { AgentViewState, RunUsage } from "@/plugins/sdk/types/agentView";
import { appendTimelineEntry, patchRun } from "@/plugins/sdk";
import { settlePendingInterrupts } from "./fold";
import { materializeInterrupt } from "./interruptMaterialization";

export function onRunStarted(
  state: AgentViewState,
  run: RunRef,
  segmentId?: string,
): AgentViewState {
  // Subagent runs share the parent stream but must not reset the root run state.
  if (run.spawnedByItemId) {
    return appendTimelineEntry({ kind: "run-start", runId: run.id, summary: "subagent" })(state);
  }
  // The per-segment streaming readout (usage/error) resets on a NEW segment, not
  // on a new run: a resume opens a fresh segment of the SAME run, so runId (from
  // run.id) stays stable while segmentId turns over. Keying the reset on
  // segmentId also makes a reconnect replay of the CURRENT segment's segment.started
  // idempotent — re-seeing the same segmentId must not wipe the live readout.
  // (A synthetic segment.started with no segmentId — reconnect scaffolding, history
  // replay — always resets, matching the prior per-segment.started behaviour.)
  const sameSegment = segmentId !== undefined && segmentId === state.run.segmentId;
  if (sameSegment) {
    return appendTimelineEntry({ kind: "run-start", runId: run.id })(state);
  }
  // Run boundaries are not turn boundaries. User items open new turns; assistant
  // items append to the current one so live streaming matches history replay.
  const next: AgentViewState = {
    ...state,
    error: null,
    run: {
      ...state.run,
      running: true,
      runId: run.id,
      segmentId: segmentId ?? null,
      sessionId: run.sessionId,
      usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
    },
  };
  return appendTimelineEntry({ kind: "run-start", runId: run.id })(next);
}

function mapUsage(u: Usage): RunUsage {
  return {
    inputTokens: u.inputTokens ?? 0,
    outputTokens: u.outputTokens ?? 0,
    cacheReadTokens: u.cacheReadTokens ?? 0,
    ...(u.costUsd !== undefined ? { costUsd: u.costUsd } : {}),
  };
}

// runMetrics is the readout patch a Run's committed accounting produces. Both the
// step counter and the total read the same field, because a Run's metrics ARE its
// total — the step preview on segment.progress is the same number arriving early.
function runMetrics(state: AgentViewState, metrics: RunMetrics): Partial<AgentViewState["run"]> {
  return {
    step: metrics.steps || state.run.step,
    totalSteps: metrics.steps || state.run.totalSteps,
    ...(metrics.usage ? { usage: mapUsage(metrics.usage) } : {}),
  };
}

export function onRunProgress(
  state: AgentViewState,
  progress: RunProgress,
  runId?: string,
): AgentViewState {
  // A subagent progress event must not overwrite the root run readout.
  if (runId && state.run.runId && runId !== state.run.runId) return state;
  return patchRun({
    ...(progress.step !== undefined ? { step: progress.step } : {}),
    ...(progress.activity !== undefined ? { activity: progress.activity } : {}),
    ...(progress.usage ? { usage: mapUsage(progress.usage) } : {}),
    ...(progress.contextTokens !== undefined ? { contextTokens: progress.contextTokens } : {}),
  })(state);
}

export function onRunFinished(
  state: AgentViewState,
  outcome: SegmentOutcome,
  metrics: RunMetrics,
  runId?: string,
): AgentViewState {
  // The wire envelope runId is the only discriminator for subagent endings.
  // Child runs affect the timeline but not the root running/interrupt state.
  if (runId && state.run.runId && runId !== state.run.runId) {
    return appendTimelineEntry({
      kind: "run-end",
      runId,
      status: outcome.type === "completed" ? "ok" : undefined,
      summary: "subagent",
    })(state);
  }
  const idle: AgentViewState = { ...state, run: { ...state.run, running: false } };
  if (outcome.type === "suspended") {
    // Another run in this tree raised the interrupt; this one only stopped. Its
    // pending set stays where it is — the run that raised it reports it.
    return patchRun({ running: false, ...runMetrics(state, metrics) })(idle);
  }
  if (outcome.type === "interrupt") {
    // The stable Run to resume — a resume continues THIS run (new segment),
    // never a fresh run.
    const interruptedRunId = state.run.runId ?? "";
    // Re-delivery can present interrupts already enrolled. Only add fresh
    // envelopes; materialization below is an idempotent upsert.
    const openItemIds = new Set(
      idle.pendingInterrupts.flatMap((oi) => oi.interrupts.map((i) => i.itemId)),
    );
    const fresh = outcome.interrupts.filter((it) => !openItemIds.has(it.itemId));
    let next: AgentViewState =
      fresh.length === 0
        ? idle
        : {
            ...idle,
            pendingInterrupts: [
              ...idle.pendingInterrupts,
              {
                runId: interruptedRunId,
                sessionId: state.run.sessionId ?? "",
                interrupts: fresh.map((interrupt) => ({
                  itemId: interrupt.itemId,
                  kind: interrupt.type,
                })),
              },
            ],
          };
    for (const it of outcome.interrupts) next = materializeInterrupt(next, it, interruptedRunId);
    return patchRun(runMetrics(state, metrics))(next);
  }

  // A terminal run invalidates any still-actionable cards it owns.
  const withRun = settlePendingInterrupts(
    patchRun({ running: false, ...runMetrics(state, metrics) })(idle),
  );

  if (outcome.type === "error") {
    const errored: AgentViewState = {
      ...withRun,
      // `message` is only what the runtime actually said about this occurrence.
      // Falling back to the symbol here would put "internal_error" in front of
      // the user and leave the banner nothing to translate — the symbol travels
      // in `code`, and the banner turns it into this locale's words.
      error: {
        message: outcome.error?.detail,
        code: outcome.error?.type,
        retryAfterSeconds: outcome.error?.retryAfterSeconds,
      },
    };
    return appendTimelineEntry({
      kind: "run-error",
      status: "err",
      // The timeline is a diagnostic log, so the symbol is the better label
      // there when there's no per-occurrence detail to show.
      summary: errored.error?.message ?? errored.error?.code,
    })(errored);
  }
  const detail = "detail" in outcome ? outcome.detail : undefined;
  return appendTimelineEntry({
    kind: "run-end",
    status: outcome.type === "completed" ? "ok" : undefined,
    summary: outcome.type === "completed" ? undefined : (detail ?? outcome.type),
  })(withRun);
}
