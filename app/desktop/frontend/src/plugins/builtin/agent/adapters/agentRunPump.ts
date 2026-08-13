import { queryClient } from "@/lib/queryClient";
import {
  RpcConnectionError,
  RpcProtocolError,
  type RunEvent,
  type RunId,
  type RunRef,
  type SegmentId,
  type StreamingResult,
} from "@/rpc";
import { AGENT_SESSION_USAGE_KEY } from "../application/session/sessionUsage";
import { createRunEventBatcher } from "./runEventBatcher";

const ABORTED = Symbol("agent-run-pump.aborted");

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
  /** Read the root Run after its live segment settles. Stream terminal events are
   *  intentionally compact; runs.get is the authoritative complete RunRef. */
  readRunSnapshot?: (runId: RunId, signal: AbortSignal) => Promise<RunRef>;
  applyRunSnapshot?: (run: RunRef) => void;
  /** Reattach a run whose stream ended before the run did. null means the run is no
   *  longer attachable at all — finished, waiting on a person, or moved to another
   *  segment — after the durable projection has reconciled that transition. */
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
  readRunSnapshot,
  applyRunSnapshot,
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
          let snapshot: RunRef | undefined;
          if (readRunSnapshot && !isCancelled() && !signal.aborted) {
            try {
              const read = await settleBeforeAbort(readRunSnapshot(runId, signal), signal);
              if (read !== ABORTED) snapshot = read;
            } catch (error) {
              if (!isCancelled() && !signal.aborted && !(error instanceof RpcConnectionError)) {
                console.warn("[agent] exact run read failed:", sessionId, runId, error);
              }
            }
          }

          // A newer pump may have opened while the exact read was in flight.
          // Its stream owns the projection now, so the older RunRef cannot be
          // folded and the older finally cannot publish an idle boundary.
          if (currentPumpSequence === pumpSequence) {
            if (snapshot && !isCancelled() && !signal.aborted) applyRunSnapshot?.(snapshot);
            currentRunId = null;
            currentSegmentId = null;
            onIdle?.();
          }
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
    const iterator = events[Symbol.asyncIterator]();
    let iteratorDone = false;
    try {
      while (!signal.aborted && !isCancelled()) {
        const pendingNext = Promise.resolve(iterator.next());
        const next = await settleBeforeAbort(pendingNext, signal);
        if (next === ABORTED) {
          const lateNext = await settleWithinTurn(pendingNext);
          if (lateNext.status === "fulfilled" && lateNext.value.done) iteratorDone = true;
          return { finished: true, position };
        }
        if (next.done) {
          iteratorDone = true;
          break;
        }
        // An aborted request or a torn-down session is a deliberate stop, not a gap
        // to recover: nothing is reattached after it.
        if (isCancelled() || signal.aborted) return { finished: true, position };
        const ev = next.value;
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
      if (!isCancelled() && !signal.aborted && !(err instanceof RpcConnectionError))
        console.warn("[agent] run stream ended early:", sessionId, err);
    } finally {
      if (!iteratorDone) await disposeIterator(iterator);
    }
    return { finished, position };
  }
}

/** Settle a foreign async boundary without depending on AbortSignal support. */
function settleBeforeAbort<T>(
  operation: Promise<T>,
  signal: AbortSignal,
): Promise<T | typeof ABORTED> {
  return new Promise((resolve, reject) => {
    let settled = false;
    const onAbort = () => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", onAbort);
      resolve(ABORTED);
    };
    if (signal.aborted) onAbort();
    else signal.addEventListener("abort", onAbort, { once: true });

    void operation.then(
      (value) => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        resolve(value);
      },
      (error: unknown) => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        reject(error);
      },
    );
  });
}

async function disposeIterator<T>(iterator: AsyncIterator<T>): Promise<void> {
  try {
    const closing = iterator.return?.();
    if (closing) await settleWithinTurn(Promise.resolve(closing));
  } catch {
    // A retiring foreign iterator cannot keep the replacement generation open.
  }
}

type TurnSettlement<T> =
  { status: "fulfilled"; value: T } | { status: "rejected" } | { status: "pending" };

function settleWithinTurn<T>(operation: Promise<T>): Promise<TurnSettlement<T>> {
  return new Promise((resolve) => {
    let settled = false;
    const finish = (result: TurnSettlement<T>) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(result);
    };
    const timer = setTimeout(() => finish({ status: "pending" }), 0);
    void operation.then(
      (value) => finish({ status: "fulfilled", value }),
      () => finish({ status: "rejected" }),
    );
  });
}
