import type { OpenInterrupt, RunOutcome, RunProgress, RunRef, Usage } from "@/rpc";
import type { AgentViewState, RunUsage } from "@/protocol/run/viewState";
import { appendTimelineEntry, patchRun } from "@/plugins/sdk";
import { settleOpenInterrupts } from "./fold";
import { materializeInterrupt } from "./interruptMaterialization";

export function onRunStarted(state: AgentViewState, run: RunRef): AgentViewState {
  // Subagent runs share the parent stream but must not reset the root run state.
  if (run.spawnedByItemId) {
    return appendTimelineEntry({ kind: "run-start", runId: run.id, summary: "subagent" })(state);
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

export function onRunProgress(
  state: AgentViewState,
  progress: RunProgress,
  runId?: string,
): AgentViewState {
  // A subagent progress event must not overwrite the root run readout.
  if (runId && state.run.runId && runId !== state.run.runId) return state;
  return patchRun({
    ...(progress.step !== undefined ? { step: progress.step } : {}),
    ...(progress.maxSteps !== undefined ? { totalSteps: progress.maxSteps } : {}),
    ...(progress.activity !== undefined ? { activity: progress.activity } : {}),
    ...(progress.usage ? { usage: mapUsage(progress.usage) } : {}),
    ...(progress.contextTokens !== undefined ? { contextTokens: progress.contextTokens } : {}),
  })(state);
}

export function onRunFinished(
  state: AgentViewState,
  outcome: RunOutcome,
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
  if (outcome.type === "interrupt") {
    const parentRunId = (state.run.runId ?? "") as OpenInterrupt["parentRunId"];
    // Re-delivery can present interrupts already enrolled. Only add fresh
    // envelopes; materialization below is an idempotent upsert.
    const openItemIds = new Set(
      idle.openInterrupts.flatMap((oi) => oi.interrupts.map((i) => i.itemId)),
    );
    const fresh = outcome.interrupts.filter((it) => !openItemIds.has(it.itemId));
    let next: AgentViewState =
      fresh.length === 0
        ? idle
        : {
            ...idle,
            openInterrupts: [
              ...idle.openInterrupts,
              {
                parentRunId,
                sessionId: (state.run.sessionId ?? "") as OpenInterrupt["sessionId"],
                interrupts: fresh,
                createdAt: new Date().toISOString(),
              },
            ],
          };
    for (const it of outcome.interrupts) next = materializeInterrupt(next, it, parentRunId);
    return next;
  }

  const { result } = outcome;
  const usage = result?.usage;
  // A terminal non-interrupt run invalidates any still-actionable cards it owns.
  const withRun = settleOpenInterrupts(
    patchRun({
      running: false,
      step: result?.steps ?? state.run.step,
      totalSteps: result?.steps ?? state.run.totalSteps,
      ...(usage ? { usage: mapUsage(usage) } : {}),
    })(idle),
  );

  if (outcome.type === "error") {
    const errored: AgentViewState = {
      ...withRun,
      error: {
        message: result?.error?.detail ?? result?.error?.type ?? "run failed",
        code: result?.error?.type,
        retryable: result?.error?.retryable,
        retryAfterSeconds: result?.error?.retryAfterSeconds,
      },
    };
    return appendTimelineEntry({
      kind: "run-error",
      status: "err",
      summary: errored.error?.message,
    })(errored);
  }
  const detail = "detail" in outcome ? outcome.detail : undefined;
  return appendTimelineEntry({
    kind: "run-end",
    status: outcome.type === "completed" ? "ok" : undefined,
    summary: outcome.type === "completed" ? undefined : (detail ?? outcome.type),
  })(withRun);
}
