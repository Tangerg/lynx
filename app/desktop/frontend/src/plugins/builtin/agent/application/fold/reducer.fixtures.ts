import type {
  AgentEventEnvelope as RunEvent,
  AgentRunFact as RunRef,
  AgentSegmentOutcome as SegmentOutcome,
  AgentStreamEvent as StreamEvent,
} from "@/plugins/sdk";
import type {
  AgentRunMetrics as RunMetrics,
  AgentSessionView,
} from "@/plugins/sdk/types/agentSessionView";
import { selectCurrentRootRun } from "../view/runTree";
import { reduceAgentEvent } from "./reducer";

/** The accounting of a run that has reported none. */
export const noMetrics: RunMetrics = {
  steps: 0,
  activeDurationMillis: 0,
  usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
};

let nextEventSequence = 0;

function completeStartedRun(state: AgentSessionView, run: RunRef, segmentId: string): RunRef {
  const parent = run.spawnedByItemId ? selectCurrentRootRun(state) : null;
  const { createdAt = "2026-06-03T00:00:00.000Z", metrics = noMetrics, status = "running" } = run;
  const child = run.spawnedByItemId !== null && run.spawnedByItemId !== undefined;
  return {
    id: run.id,
    sessionId: run.sessionId,
    parentRunId: child ? (run.parentRunId ?? parent?.id ?? null) : null,
    rootRunId: child ? (run.rootRunId ?? parent?.rootRunId ?? parent?.id ?? run.id) : run.id,
    spawnedByItemId: run.spawnedByItemId ?? null,
    activeSegmentId: segmentId,
    createdAt,
    metrics,
    status,
    outcome: null,
    finishedAt: null,
  };
}

/**
 * Build a complete product-owned Agent envelope for scenario tests.
 *
 * Production folds never infer provenance: they accept AgentEventEnvelope only. This
 * test helper keeps the scenario bodies readable while stamping every payload
 * with an explicit, internally consistent source. Source-sensitive contracts
 * should call `testRunEvent` directly and assert the envelope fields.
 */
export function testRunEvent(
  state: AgentSessionView,
  event: StreamEvent,
  runId?: string,
  segmentId?: string,
): RunEvent {
  const payloadRunId =
    event.type === "segment.started"
      ? event.run.id
      : event.type === "item.started" || event.type === "item.completed"
        ? event.item.runId
        : undefined;
  const ownerRunId = runId ?? payloadRunId ?? selectCurrentRootRun(state)?.id ?? "run_1";
  const owner = state.runsById[ownerRunId];
  const ownerSegmentId =
    segmentId ??
    (event.type === "segment.started" ? event.run.activeSegmentId : owner?.activeSegmentId) ??
    `seg_${ownerRunId}`;
  const sequence = ++nextEventSequence;
  const normalizedEvent: StreamEvent =
    event.type === "segment.started"
      ? {
          ...event,
          run: completeStartedRun(state, event.run, ownerSegmentId),
        }
      : event;
  return {
    event: normalizedEvent,
    eventId: `evt_test_${sequence}`,
    runId: ownerRunId,
    segmentId: ownerSegmentId,
    timestamp: `2026-06-03T00:00:${String(sequence % 60).padStart(2, "0")}.000Z`,
  };
}

export function foldTestEvent(
  state: AgentSessionView,
  event: StreamEvent,
  runId?: string,
  segmentId?: string,
): AgentSessionView {
  return reduceAgentEvent(state, testRunEvent(state, event, runId, segmentId));
}

/**
 * runFinished builds a `segment.finished` frame.
 *
 * Every reducer suite needs one and they all needed the same one, so it lives
 * here: metrics ride the frame beside the outcome now, and eight copies of the
 * builder is eight places to remember that.
 */
export const runFinished = (
  outcome: SegmentOutcome,
  metrics: Omit<RunMetrics, "usage"> & { usage?: RunMetrics["usage"] } = noMetrics,
): StreamEvent => ({
  type: "segment.finished",
  outcome,
  metrics: { ...metrics, usage: metrics.usage ?? noMetrics.usage },
});
