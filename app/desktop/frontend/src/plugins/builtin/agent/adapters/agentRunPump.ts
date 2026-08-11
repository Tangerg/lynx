import { queryClient } from "@/lib/queryClient";
import {
  RpcProtocolError,
  type RunEvent,
  type RunId,
  type SegmentId,
  type StreamingResult,
} from "@/rpc";
import { AGENT_SESSION_USAGE_KEY } from "../application/session/sessionUsage";
import { createRunEventBatcher } from "./runEventBatcher";

/** What a stream's opening ack tells the pump. headEventId exists only on a
 *  reattach: a start or resume stream begins at the beginning of its segment, so
 *  there is no earlier position to name. */
export interface RunStreamAck {
  runId: RunId;
  segmentId: SegmentId;
  headEventId?: string;
}

export type RunStream = StreamingResult<RunStreamAck, RunEvent>;

/** Where a reattach picks up. lastEventId is empty when this client has folded
 *  nothing and was given no head — then the reattach is tail-only and the
 *  durable session snapshot supplies the materialized projection. */
export interface RunStreamPosition {
  runId: RunId;
  segmentId: SegmentId;
  lastEventId: string;
  recovery: "replay" | "cold";
}

interface AgentRunPumpOptions {
  sessionId: string;
  isCancelled: () => boolean;
  readEpoch: () => number;
  applyEvents: (events: RunEvent[]) => void;
  /** Reattach a run whose stream ended before the run did. null means the run is no
   *  longer attachable at all — finished, waiting on a person, or moved to another
   *  segment — and the fold already holds, or will be told, everything it can. */
  reattach?: (position: RunStreamPosition, signal: AbortSignal) => Promise<RunStream | null>;
  /** The newest live stream became idle after its queued tail was folded. */
  onIdle?: () => void;
}

interface AgentRunPump {
  pump: (stream: RunStream, signal: AbortSignal) => Promise<void>;
  isFollowing: (runId: string, segmentId: string) => boolean;
  isActive: () => boolean;
  dispose: () => void;
}

export function createAgentRunPump({
  sessionId,
  isCancelled,
  readEpoch,
  applyEvents,
  reattach,
  onIdle,
}: AgentRunPumpOptions): AgentRunPump {
  let currentRunId: RunId | null = null;
  let currentSegmentId: SegmentId | null = null;
  let currentPumpSequence = 0;

  const eventBatcher = createRunEventBatcher({
    readEpoch,
    apply: applyEvents,
    onRunFinished: () => {
      void queryClient.invalidateQueries({ queryKey: [AGENT_SESSION_USAGE_KEY, sessionId] });
    },
  });

  return {
    // A run outlives its stream. Ending without the segment's own terminal is an
    // abnormal EOS — a dropped connection, not a finished run — and the run keeps
    // executing on the server either way. Reattaching from the last folded event is
    // what turns that into a gap of milliseconds instead of a transcript frozen until
    // the next reload.
    async pump(stream, signal) {
      const pumpSequence = ++currentPumpSequence;
      const runId = stream.result.runId;
      currentRunId = runId;
      currentSegmentId = stream.result.segmentId;
      let position: RunStreamPosition = {
        runId,
        segmentId: stream.result.segmentId,
        lastEventId: stream.result.headEventId ?? "",
        recovery: "replay",
      };
      let events: AsyncIterable<RunEvent> | null = stream.events;
      try {
        while (events) {
          const drained = await consume(events, position, signal);
          position = drained.position;
          if (drained.finished || !reattach || isCancelled() || signal.aborted) break;
          const next = await reattach(position, signal);
          if (!next) break;
          position = {
            runId,
            segmentId: next.result.segmentId,
            // Only adopt the ack's head when this client holds no cursor of its own:
            // the head of a replaying attach sits AHEAD of what was asked for, so
            // taking it would silently skip everything the replay is delivering.
            lastEventId: position.lastEventId || (next.result.headEventId ?? ""),
            recovery: "replay",
          };
          currentSegmentId = next.result.segmentId;
          events = next.events;
        }
      } finally {
        if (currentPumpSequence === pumpSequence) {
          // The durable change stream may already have requested a projection
          // refresh. Land this stream's rAF-delayed tail before declaring the
          // session idle, otherwise the newer snapshot can overtake it.
          eventBatcher.flush();
          currentRunId = null;
          currentSegmentId = null;
          onIdle?.();
        }
      }
    },
    isFollowing(runId, segmentId) {
      return currentRunId === runId && currentSegmentId === segmentId;
    },
    isActive() {
      return currentRunId !== null;
    },
    dispose() {
      eventBatcher.dispose();
    },
  };

  async function consume(
    events: AsyncIterable<RunEvent>,
    from: RunStreamPosition,
    signal: AbortSignal,
  ): Promise<{ finished: boolean; position: RunStreamPosition }> {
    let position = from;
    let finished = false;
    try {
      for await (const ev of events) {
        // An aborted request or a torn-down session is a deliberate stop, not a gap
        // to recover: nothing is reattached after it.
        if (isCancelled() || signal.aborted) return { finished: true, position };
        eventBatcher.enqueue(ev);
        position = { ...position, lastEventId: ev.eventId };
        // A descendant subagent's terminal rides this same stream; only the root
        // segment's ends it.
        if (ev.segmentId === position.segmentId && ev.event.type === "segment.finished") {
          finished = true;
        }
      }
    } catch (err) {
      if (err instanceof RpcProtocolError) {
        position = { ...position, recovery: "cold" };
      }
      if (!isCancelled() && !signal.aborted)
        console.warn("[agent] run stream ended early:", sessionId, err);
    }
    return { finished, position };
  }
}
