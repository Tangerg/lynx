import { describe, expect, it, vi } from "vitest";
import type { RunEvent } from "@/rpc";
import { createRunEventBatcher } from "./runEventBatcher";

let sequence = 0;
const envelope = (event: RunEvent["event"]): RunEvent => ({
  event,
  eventId: `evt_batch_${++sequence}`,
  runId: "run_1",
  segmentId: "seg_1",
  timestamp: "2026-06-03T00:00:00.000Z",
});

const runStarted = (): RunEvent =>
  envelope({
    type: "segment.started",
    run: {
      id: "run_1",
      sessionId: "ses_1",
      status: "running",
      activeSegmentId: "seg_1",
      createdAt: "2026-06-03T00:00:00.000Z",
      metrics: { steps: 0, activeDurationMillis: 0 },
      protocolProfile: { interruptTypes: [], requiredFeatures: [] },
    },
  });

// A segment.finished frame carries both halves the wire requires: why it stopped
// and what the run consumed.
const runFinished = (): RunEvent =>
  envelope({
    type: "segment.finished",
    outcome: { type: "completed" },
    metrics: { steps: 0, activeDurationMillis: 1 },
  });

function frameScheduler() {
  const scheduled: Array<() => void> = [];
  const scheduleFrame = vi.fn((flush: () => void) => {
    scheduled.push(flush);
    return scheduled.length;
  });
  const cancelFrame = vi.fn();
  return {
    scheduleFrame,
    cancelFrame,
    flushNext: () => scheduled.shift()?.(),
  };
}

describe("createRunEventBatcher", () => {
  it("coalesces queued events into one frame and reports finished runs", () => {
    const applied: RunEvent[][] = [];
    const onRunFinished = vi.fn();
    const frames = frameScheduler();
    const batcher = createRunEventBatcher({
      readEpoch: () => 0,
      apply: (batch) => applied.push(batch),
      onRunFinished,
      scheduleFrame: frames.scheduleFrame,
      cancelFrame: frames.cancelFrame,
    });

    batcher.enqueue(runStarted());
    batcher.enqueue(runFinished());

    expect(frames.scheduleFrame).toHaveBeenCalledTimes(1);
    expect(applied).toEqual([]);

    frames.flushNext();

    expect(applied).toHaveLength(1);
    expect(applied[0]!.map((entry) => entry.event.type)).toEqual([
      "segment.started",
      "segment.finished",
    ]);
    expect(applied[0]![1]!.runId).toBe("run_1");
    expect(onRunFinished).toHaveBeenCalledTimes(1);
  });

  it("drops a queued batch when the view epoch changes before flush", () => {
    let epoch = 1;
    const applied: RunEvent[][] = [];
    const frames = frameScheduler();
    const batcher = createRunEventBatcher({
      readEpoch: () => epoch,
      apply: (batch) => applied.push(batch),
      scheduleFrame: frames.scheduleFrame,
      cancelFrame: frames.cancelFrame,
    });

    batcher.enqueue(runStarted());
    epoch = 2;
    frames.flushNext();

    expect(applied).toEqual([]);

    batcher.enqueue(runFinished());
    frames.flushNext();

    expect(applied).toHaveLength(1);
    expect(applied[0]![0]!.event.type).toBe("segment.finished");
  });

  it("can land the queued tail synchronously before a durable snapshot", () => {
    const applied: RunEvent[][] = [];
    const frames = frameScheduler();
    const batcher = createRunEventBatcher({
      readEpoch: () => 0,
      apply: (batch) => applied.push(batch),
      scheduleFrame: frames.scheduleFrame,
      cancelFrame: frames.cancelFrame,
    });

    batcher.enqueue(runStarted());
    batcher.enqueue(runFinished());
    batcher.flush();

    expect(frames.cancelFrame).toHaveBeenCalledWith(1);
    expect(applied).toHaveLength(1);
    expect(applied[0]!.map((entry) => entry.event.type)).toEqual([
      "segment.started",
      "segment.finished",
    ]);

    // A frame already queued by the browser is harmless if its callback races
    // the cancellation and still fires.
    frames.flushNext();
    expect(applied).toHaveLength(1);
  });

  it("cancels pending frames and ignores future events after dispose", () => {
    const applied: RunEvent[][] = [];
    const frames = frameScheduler();
    const batcher = createRunEventBatcher({
      readEpoch: () => 0,
      apply: (batch) => applied.push(batch),
      scheduleFrame: frames.scheduleFrame,
      cancelFrame: frames.cancelFrame,
    });

    batcher.enqueue(runStarted());
    batcher.dispose();
    batcher.enqueue(runFinished());
    frames.flushNext();

    expect(frames.cancelFrame).toHaveBeenCalledWith(1);
    expect(frames.scheduleFrame).toHaveBeenCalledTimes(1);
    expect(applied).toEqual([]);
  });
});
