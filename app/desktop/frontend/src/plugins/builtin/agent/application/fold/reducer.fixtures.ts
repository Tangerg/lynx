import type { RunEvent, RunMetrics, RunRef, SegmentOutcome, StreamEvent } from "@/rpc";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { selectCurrentRootRun, selectRun } from "../view/runTree";
import { reduceRunEvent } from "./reducer";

/** The accounting of a run that has reported none. */
export const noMetrics: RunMetrics = { steps: 0, activeDurationMillis: 0 };

let nextEventSequence = 0;

function completeStartedRun(state: AgentSessionView, run: RunRef, segmentId: string): RunRef {
  const parent = run.spawnedByItemId ? selectCurrentRootRun(state) : null;
  const {
    createdAt = "2026-06-03T00:00:00.000Z",
    metrics = noMetrics,
    protocolProfile = { interruptTypes: [], requiredFeatures: [] },
    status = "running",
    ...identity
  } = run;
  return {
    ...identity,
    activeSegmentId: segmentId,
    createdAt,
    metrics,
    protocolProfile,
    status,
    ...(run.spawnedByItemId
      ? {
          parentRunId: run.parentRunId ?? parent?.id,
          rootRunId: run.rootRunId ?? parent?.rootRunId,
        }
      : {}),
  };
}

/**
 * Build a complete wire envelope for scenario tests.
 *
 * Production folds never infer provenance: they accept RunEvent only. This
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
  const owner = selectRun(state, ownerRunId);
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
  return reduceRunEvent(state, testRunEvent(state, event, runId, segmentId));
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
  metrics: RunMetrics = noMetrics,
): StreamEvent => ({ type: "segment.finished", outcome, metrics });
