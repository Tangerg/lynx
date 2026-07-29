import type { RunMetrics, SegmentOutcome, StreamEvent } from "@/rpc";

/** The accounting of a run that has reported none. */
export const noMetrics: RunMetrics = { steps: 0, activeDurationMs: 0 };

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
