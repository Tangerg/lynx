import { queryClient } from "@/lib/queryClient";
import type { RunEvent, RunId, SegmentId, StreamingResult } from "@/rpc";
import { AGENT_SESSION_USAGE_KEY } from "../application/session/sessionUsage";
import type { FoldEvent } from "./agentStore";
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
 *  nothing and was given no head — then the reattach is tail-only and the history
 *  comes from items.list, which is the only place it authoritatively lives. */
export interface RunStreamPosition {
  runId: RunId;
  segmentId: SegmentId;
  lastEventId: string;
}

interface AgentRunPumpOptions {
  sessionId: string;
  isCancelled: () => boolean;
  readEpoch: () => number;
  applyEvents: (events: FoldEvent[]) => void;
  /** Reattach a run whose stream ended before the run did. null means the run is no
   *  longer attachable at all — finished, waiting on a person, or moved to another
   *  segment — and the fold already holds, or will be told, everything it can. */
  reattach?: (position: RunStreamPosition, signal: AbortSignal) => Promise<RunStream | null>;
}

interface AgentRunPump {
  pump: (stream: RunStream, signal: AbortSignal) => Promise<void>;
  cancelCurrentRun: (cancel: (runId: RunId) => Promise<void>) => void;
  dispose: () => void;
}

export function createAgentRunPump({
  sessionId,
  isCancelled,
  readEpoch,
  applyEvents,
  reattach,
}: AgentRunPumpOptions): AgentRunPump {
  let currentRunId: RunId | null = null;

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
      const runId = stream.result.runId;
      currentRunId = runId;
      let position: RunStreamPosition = {
        runId,
        segmentId: stream.result.segmentId,
        lastEventId: stream.result.headEventId ?? "",
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
            ...position,
            segmentId: next.result.segmentId,
            // Only adopt the ack's head when this client holds no cursor of its own:
            // the head of a replaying attach sits AHEAD of what was asked for, so
            // taking it would silently skip everything the replay is delivering.
            lastEventId: position.lastEventId || (next.result.headEventId ?? ""),
          };
          events = next.events;
        }
      } finally {
        if (currentRunId === runId) currentRunId = null;
      }
    },
    cancelCurrentRun(cancel) {
      if (currentRunId) void cancel(currentRunId).catch(() => undefined);
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
        eventBatcher.enqueue(ev.event, ev.runId, ev.segmentId);
        position = { ...position, lastEventId: ev.eventId };
        // A descendant subagent's terminal rides this same stream; only the root
        // segment's ends it.
        if (ev.segmentId === position.segmentId && ev.event.type === "segment.finished") {
          finished = true;
        }
      }
    } catch (err) {
      if (!isCancelled() && !signal.aborted)
        console.warn("[agent] run stream ended early:", sessionId, err);
    }
    return { finished, position };
  }
}
