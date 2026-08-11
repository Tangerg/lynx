// The pump keeps a run attached, not a stream open. These lock the difference: a
// stream that ends without the segment's own terminal is a dropped connection, and
// the run behind it is still executing.

import { describe, expect, it, vi } from "vitest";
import type { RunEvent } from "@/rpc";
import { asRunId, asSegmentId, RpcProtocolError } from "@/rpc";
import { createAgentRunPump, type RunStream, type RunStreamPosition } from "./agentRunPump";

const RUN = asRunId("run_1");
const SEGMENT = asSegmentId("seg_1");

function frame(eventId: string, event: RunEvent["event"], segmentId = SEGMENT): RunEvent {
  return {
    runId: RUN,
    segmentId,
    eventId,
    timestamp: "2026-07-29T00:00:00Z",
    event,
  } as RunEvent;
}

const progressed: RunEvent["event"] = { type: "segment.progress", progress: { step: 1 } } as never;
const finished: RunEvent["event"] = {
  type: "segment.finished",
  outcome: { type: "completed" },
  metrics: { steps: 1, activeDurationMillis: 1 },
} as never;

function streamOf(events: RunEvent[], headEventId?: string): RunStream {
  return {
    result: { runId: RUN, segmentId: SEGMENT, ...(headEventId ? { headEventId } : {}) },
    events: (async function* () {
      for (const ev of events) yield ev;
    })(),
  };
}

function pumpWith(reattach: (position: RunStreamPosition) => Promise<RunStream | null>) {
  const positions: RunStreamPosition[] = [];
  const pump = createAgentRunPump({
    sessionId: "ses_1",
    isCancelled: () => false,
    readEpoch: () => 0,
    applyEvents: vi.fn(),
    reattach: (position) => {
      positions.push(position);
      return reattach(position);
    },
  });
  return { pump, positions };
}

describe("agent run pump reattach", () => {
  it("resumes from the last event it folded when a stream ends without a terminal", async () => {
    const { pump, positions } = pumpWith(() =>
      Promise.resolve(streamOf([frame("evt_9", finished)])),
    );

    await pump.pump(streamOf([frame("evt_7", progressed)]), new AbortController().signal);

    expect(positions).toHaveLength(1);
    expect(positions[0]).toMatchObject({
      runId: RUN,
      segmentId: SEGMENT,
      lastEventId: "evt_7",
      recovery: "replay",
    });
  });

  it("requests cold recovery after an authoritative protocol violation", async () => {
    const failed: RunStream = {
      result: { runId: RUN, segmentId: SEGMENT },
      events: (async function* () {
        yield frame("evt_7", progressed);
        throw new RpcProtocolError("notifications.run.event params", [
          { path: "event.item", detail: "is required" },
        ]);
      })(),
    };
    const { pump, positions } = pumpWith(() =>
      Promise.resolve(streamOf([frame("evt_9", finished)])),
    );

    await pump.pump(failed, new AbortController().signal);

    expect(positions[0]).toMatchObject({ lastEventId: "evt_7", recovery: "cold" });
  });

  it("hands back the head the attach captured when it folded nothing", async () => {
    const { pump, positions } = pumpWith(() =>
      Promise.resolve(streamOf([frame("evt_5", finished)])),
    );

    await pump.pump(streamOf([], "evt_head"), new AbortController().signal);

    expect(positions[0]?.lastEventId).toBe("evt_head");
  });

  it("stops at the segment's own terminal", async () => {
    const { pump, positions } = pumpWith(() => Promise.resolve(null));

    await pump.pump(
      streamOf([frame("evt_1", progressed), frame("evt_2", finished)]),
      new AbortController().signal,
    );

    expect(positions).toHaveLength(0);
  });

  it("keeps its own cursor across a replaying reattach", async () => {
    // The ack of a reattach reports the head as of that attach, which is AHEAD of the
    // cursor being replayed from. Adopting it would skip the replay.
    let attempt = 0;
    const { pump, positions } = pumpWith(() => {
      attempt += 1;
      return Promise.resolve(
        attempt === 1
          ? streamOf([], "evt_head_ahead")
          : streamOf([frame("evt_terminal", finished)]),
      );
    });

    await pump.pump(streamOf([frame("evt_3", progressed)]), new AbortController().signal);

    expect(positions.map((p) => p.lastEventId)).toEqual(["evt_3", "evt_3"]);
  });

  it("gives up when the run is no longer attachable", async () => {
    const { pump, positions } = pumpWith(() => Promise.resolve(null));

    await pump.pump(streamOf([frame("evt_4", progressed)]), new AbortController().signal);

    expect(positions).toHaveLength(1);
  });

  it("does not reattach a stream the caller aborted", async () => {
    const { pump, positions } = pumpWith(() => Promise.resolve(null));
    const ctrl = new AbortController();
    ctrl.abort();

    await pump.pump(streamOf([frame("evt_6", progressed)]), ctrl.signal);

    expect(positions).toHaveLength(0);
  });

  it("does not let an older pump clear a newer segment for the same run", async () => {
    const pump = createAgentRunPump({
      sessionId: "ses_1",
      isCancelled: () => false,
      readEpoch: () => 0,
      applyEvents: vi.fn(),
    });
    const oldController = new AbortController();
    const newController = new AbortController();
    const oldSegment = asSegmentId("seg_old");
    const newSegment = asSegmentId("seg_new");

    const older = pump.pump(parkedStream(oldSegment, oldController.signal), oldController.signal);
    await Promise.resolve();
    const newer = pump.pump(parkedStream(newSegment, newController.signal), newController.signal);
    await Promise.resolve();
    expect(pump.isFollowing(RUN, newSegment)).toBe(true);

    oldController.abort();
    await older;
    expect(pump.isFollowing(RUN, newSegment)).toBe(true);

    newController.abort();
    await newer;
    expect(pump.isFollowing(RUN, newSegment)).toBe(false);
  });
});

function parkedStream(segmentId: ReturnType<typeof asSegmentId>, signal: AbortSignal): RunStream {
  return {
    result: { runId: RUN, segmentId },
    events: {
      [Symbol.asyncIterator]() {
        return {
          async next(): Promise<IteratorResult<RunEvent>> {
            await new Promise<never>((_, reject) => {
              if (signal.aborted) {
                reject(new Error("aborted"));
                return;
              }
              signal.addEventListener("abort", () => reject(new Error("aborted")), { once: true });
            });
            return { done: true, value: undefined as never };
          },
        };
      },
    },
  };
}
