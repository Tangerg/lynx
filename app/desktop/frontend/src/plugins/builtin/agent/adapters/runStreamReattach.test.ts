import { describe, expect, it, vi } from "vitest";
import type { LyraClient } from "@/rpc";
import { asRunId, asSegmentId } from "@/rpc";
import type { RunStream, RunStreamPosition } from "./agentRunPump";
import { createRunStreamReattach } from "./runStreamReattach";

const RUN = asRunId("run_1");
const SEGMENT = asSegmentId("seg_1");

function emptyStream(): RunStream {
  return {
    result: { runId: RUN, segmentId: SEGMENT },
    events: (async function* () {})(),
  };
}

function position(recovery: RunStreamPosition["recovery"]): RunStreamPosition {
  return {
    runId: RUN,
    segmentId: SEGMENT,
    lastEventId: "evt_7",
    recovery,
  };
}

function runClient(subscribe: LyraClient["runs"]["subscribe"]): Pick<LyraClient, "runs"> {
  return { runs: { subscribe } as LyraClient["runs"] };
}

describe("run stream reattach", () => {
  it("rebuilds the durable projection before tailing after a protocol violation", async () => {
    const order: string[] = [];
    const recoverProjection = vi.fn(async () => {
      order.push("recover");
    });
    const subscribe = vi.fn<LyraClient["runs"]["subscribe"]>(async () => {
      order.push("subscribe");
      return emptyStream();
    });
    const reattach = createRunStreamReattach({
      sessionId: "ses_1",
      client: () => runClient(subscribe),
      isCancelled: () => false,
      recoverProjection,
    });
    const signal = new AbortController().signal;

    await expect(reattach(position("cold"), signal)).resolves.not.toBeNull();

    expect(order).toEqual(["recover", "subscribe"]);
    expect(subscribe).toHaveBeenCalledWith({ runId: RUN, segmentId: SEGMENT }, signal);
  });

  it("replays from the last folded event while the cursor remains trustworthy", async () => {
    const recoverProjection = vi.fn(async () => {});
    const subscribe = vi.fn<LyraClient["runs"]["subscribe"]>(async () => emptyStream());
    const reattach = createRunStreamReattach({
      sessionId: "ses_1",
      client: () => runClient(subscribe),
      isCancelled: () => false,
      recoverProjection,
    });
    const signal = new AbortController().signal;

    await expect(reattach(position("replay"), signal)).resolves.not.toBeNull();

    expect(recoverProjection).not.toHaveBeenCalled();
    expect(subscribe).toHaveBeenCalledWith({ runId: RUN, segmentId: SEGMENT }, signal, {
      lastEventId: "evt_7",
    });
  });
});
