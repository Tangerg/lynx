import type { RunEvent, RunId, SegmentId, StreamingResult } from "@/rpc";
import type { AgentProblem } from "@/plugins/sdk/types/agentSessionView";
import { endSpan, startRunSpan, withSpan } from "@/lib/observability/tracing";
import { agentProblemFromRpcFailure } from "./rpcProblem";

/** A Run segment this client opened and can immediately pump. */
export type RunOpening = { runId: RunId; segmentId: SegmentId };

interface RunOpeningControllerOptions {
  sessionId: string;
  isCancelled: () => boolean;
  markInteracted: () => void;
  setAbortController: (controller: AbortController) => void;
  abortCurrent: () => void;
  pump: (stream: StreamingResult<RunOpening, RunEvent>, signal: AbortSignal) => Promise<void>;
  setStartError: (error: AgentProblem) => void;
}

export interface RunOpeningController {
  isStarting: () => boolean;
  begin: <Result extends RunOpening>(
    run: (signal: AbortSignal) => Promise<StreamingResult<Result, RunEvent>>,
    onResult?: (result: Result) => void,
    onStartError?: () => boolean | void,
  ) => void;
  /** Supersede the current opening/stream generation synchronously. */
  retire: () => void;
}

export function createRunOpeningController({
  sessionId,
  isCancelled,
  markInteracted,
  setAbortController,
  abortCurrent,
  pump,
  setStartError,
}: RunOpeningControllerOptions): RunOpeningController {
  let starting = false;
  let beginSeq = 0;
  let endActiveSpan: (() => void) | null = null;

  return {
    isStarting: () => starting,
    begin(run, onResult, onStartError) {
      endActiveSpan?.();
      starting = true;
      const beginId = ++beginSeq;
      markInteracted();
      abortCurrent();
      const ctrl = new AbortController();
      setAbortController(ctrl);
      const span = startRunSpan({ "scopeapp.session_id": sessionId });
      let failure: unknown;
      let spanEnded = false;
      const finishSpan = () => {
        if (spanEnded) return;
        spanEnded = true;
        if (endActiveSpan === finishSpan) endActiveSpan = null;
        endSpan(span, failure);
      };
      endActiveSpan = finishSpan;
      let opening: ReturnType<typeof run>;
      try {
        opening = withSpan(span, () => run(ctrl.signal));
      } catch (err) {
        opening = Promise.reject(err);
      }
      void opening
        .then(
          async (stream) => {
            if (isCancelled() || ctrl.signal.aborted || beginId !== beginSeq) {
              disposeIterable(stream.events);
              return;
            }
            try {
              onResult?.(stream.result);
              span.setAttribute("scopeapp.run_id", stream.result.runId);
              await pump(stream, ctrl.signal);
            } catch (err) {
              if (isCancelled() || ctrl.signal.aborted || beginId !== beginSeq) return;
              failure = err;
              console.error("[agent] accepted run stream failed:", sessionId, err);
            }
          },
          (err: unknown) => {
            if (isCancelled() || ctrl.signal.aborted || beginId !== beginSeq) return;
            // The Application projection may already prove that another client
            // won this opening race (for example, consumed the same HITL set).
            // Let that neutral fact suppress a now-stale command error without
            // teaching this Adapter any operation-specific wire error types.
            if (onStartError?.() === true) return;
            failure = err;
            console.error("[agent] run failed to start:", sessionId, err);
            const problem = agentProblemFromRpcFailure(err);
            if (problem) setStartError(problem);
          },
        )
        .finally(() => {
          if (beginId === beginSeq) starting = false;
          finishSpan();
        });
    },
    retire() {
      beginSeq += 1;
      starting = false;
      abortCurrent();
      endActiveSpan?.();
    },
  };
}

function disposeIterable<T>(iterable: AsyncIterable<T>): void {
  try {
    const closing = iterable[Symbol.asyncIterator]().return?.();
    if (closing) void Promise.resolve(closing).catch(() => undefined);
  } catch {
    // The generation is already fenced. Abort remains the authoritative
    // teardown path when a foreign iterator cannot be constructed or closed.
  }
}
