import { describe, expect, it, vi } from "vitest";
import type { RunEvent } from "@/rpc";
import type { FoldEvent } from "./agentStore";
import { createRunEventBatcher } from "./runEventBatcher";

const runStarted = (): RunEvent["event"] =>
  ({ type: "run.started", run: { id: "run_1", sessionId: "ses_1" } }) as RunEvent["event"];

const runFinished = (): RunEvent["event"] => ({
  type: "run.finished",
  outcome: { type: "completed" },
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
    const applied: FoldEvent[][] = [];
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
    batcher.enqueue(runFinished(), "run_1");

    expect(frames.scheduleFrame).toHaveBeenCalledTimes(1);
    expect(applied).toEqual([]);

    frames.flushNext();

    expect(applied).toHaveLength(1);
    expect(applied[0]!.map((entry) => entry.event.type)).toEqual(["run.started", "run.finished"]);
    expect(applied[0]![1]!.runId).toBe("run_1");
    expect(onRunFinished).toHaveBeenCalledTimes(1);
  });

  it("drops a queued batch when the view epoch changes before flush", () => {
    let epoch = 1;
    const applied: FoldEvent[][] = [];
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
    expect(applied[0]![0]!.event.type).toBe("run.finished");
  });

  it("cancels pending frames and ignores future events after dispose", () => {
    const applied: FoldEvent[][] = [];
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
