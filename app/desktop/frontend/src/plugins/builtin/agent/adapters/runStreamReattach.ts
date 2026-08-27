import type { ScopeAppClient } from "@/rpc";
import { asRunId, asSegmentId, RpcConnectionError } from "@/rpc";
import { agentRuntime } from "../application/ports/runtimeGateway";
import type { RunStream, RunStreamPosition } from "./agentRunPump";
import { retireRunStream, settleRunStreamOpening } from "./runStreamOpening";

interface RunStreamReattachOptions {
  sessionId: string;
  client: () => Pick<ScopeAppClient, "runs">;
  isCancelled: () => boolean;
  /** Rebuild the complete durable projection when the replay window no longer
   *  reaches this client's cursor. Missed deltas are gone, but their completed
   *  items and lifecycle facts remain queryable. */
  recoverProjection: (signal: AbortSignal) => Promise<void>;
}

/**
 * Reattach a run whose stream ended before the run did.
 *
 * The cursor is handed back verbatim, and the runtime either replays from just after
 * it or refuses. The two refusals mean different things and get different answers:
 *
 *   - the run is not attachable (finished, waiting on a person, or already on another
 *     segment) — there is nothing to follow, so the durable projection is re-read;
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
    const recoverAndTail = async (): Promise<RunStream | null> => {
      await recoverProjection(signal);
      if (isCancelled() || signal.aborted) return null;
      try {
        const tail = await settleRunStreamOpening(client().runs.subscribe(target, signal), signal);
        if (!tail) return null;
        if (isCancelled() || signal.aborted) {
          retireRunStream(tail);
          return null;
        }
        return { result: brandAck(tail.result), events: tail.events };
      } catch (tailErr) {
        if (
          !isCancelled() &&
          !signal.aborted &&
          !(tailErr instanceof RpcConnectionError) &&
          !agentRuntime().isRunGone(tailErr)
        )
          console.warn("[agent] run tail reattach failed:", sessionId, tailErr);
        return null;
      }
    };

    if (position.recovery === "cold") return recoverAndTail();
    try {
      const stream = await settleRunStreamOpening(
        client().runs.subscribe(target, signal, {
          ...(position.lastEventId ? { lastEventId: position.lastEventId } : {}),
        }),
        signal,
      );
      if (!stream) return null;
      if (isCancelled() || signal.aborted) {
        retireRunStream(stream);
        return null;
      }
      return { result: brandAck(stream.result), events: stream.events };
    } catch (err) {
      if (isCancelled() || signal.aborted) return null;
      if (agentRuntime().isRunGone(err)) {
        await recoverProjection(signal);
        return null;
      }
      if (err instanceof RpcConnectionError) return null;
      if (!agentRuntime().isReplayLost(err)) {
        console.warn("[agent] run reattach failed:", sessionId, err);
        return null;
      }
      return recoverAndTail();
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
