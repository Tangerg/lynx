import type { LyraClient } from "@/rpc";
import { asRunId, asSegmentId } from "@/rpc";
import { agentRuntime } from "../application/ports/runtimeGateway";
import type { RunStream, RunStreamPosition } from "./agentRunPump";

interface RunStreamReattachOptions {
  sessionId: string;
  client: () => Pick<LyraClient, "runs">;
  isCancelled: () => boolean;
  /** Rebuild the complete durable projection when the replay window no longer
   *  reaches this client's cursor. Missed deltas are gone, but their completed
   *  items and lifecycle facts remain queryable. */
  recoverProjection: () => Promise<void>;
}

/**
 * Reattach a run whose stream ended before the run did.
 *
 * The cursor is handed back verbatim, and the runtime either replays from just after
 * it or refuses. The two refusals mean different things and get different answers:
 *
 *   - the run is not attachable (finished, waiting on a person, or already on another
 *     segment) — there is nothing to follow, and the fold either holds the end
 *     already or learns it from the change stream;
 *   - the replay window has moved past the cursor — the events are unrecoverable, but
 *     the Items they produced are persisted, so the history is re-read and the stream
 *     is reattached tail-only. Attaching tail-first without that read would leave a
 *     transcript missing whatever the gap contained.
 */
export function createRunStreamReattach({
  sessionId,
  client,
  isCancelled,
  recoverProjection,
}: RunStreamReattachOptions) {
  return async function reattach(
    position: RunStreamPosition,
    signal: AbortSignal,
  ): Promise<RunStream | null> {
    if (isCancelled() || signal.aborted) return null;
    const target = {
      runId: asRunId(position.runId),
      segmentId: asSegmentId(position.segmentId),
    };
    try {
      const stream = await client().runs.subscribe(target, signal, {
        ...(position.lastEventId ? { lastEventId: position.lastEventId } : {}),
      });
      return { result: brandAck(stream.result), events: stream.events };
    } catch (err) {
      if (isCancelled() || signal.aborted) return null;
      if (agentRuntime().isRunGone(err)) return null;
      if (!agentRuntime().isReplayLost(err)) {
        console.warn("[agent] run reattach failed:", sessionId, err);
        return null;
      }
      await recoverProjection();
      if (isCancelled() || signal.aborted) return null;
      try {
        const tail = await client().runs.subscribe(target, signal);
        return { result: brandAck(tail.result), events: tail.events };
      } catch (tailErr) {
        if (!isCancelled() && !signal.aborted)
          console.warn("[agent] run tail reattach failed:", sessionId, tailErr);
        return null;
      }
    }
  };
}

// The subscribe ack is the wire's own shape, so this is the parse site for its ids —
// the same rule the gateway follows for a run it opens.
function brandAck(result: { runId: string; segmentId: string; headEventId?: string }) {
  return {
    runId: asRunId(result.runId),
    segmentId: asSegmentId(result.segmentId),
    ...(result.headEventId ? { headEventId: result.headEventId } : {}),
  };
}
