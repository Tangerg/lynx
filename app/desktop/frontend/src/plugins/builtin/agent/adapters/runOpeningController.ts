import type { RunEvent, RunId, SegmentId, StreamingResult } from "@/rpc";
import { errorDetail, errorType, RpcError } from "@/rpc";
import type { AgentProblem } from "@/plugins/sdk/types/agentSessionView";
import { endSpan, startRunSpan, withSpan } from "@/lib/observability/tracing";

/** A run this client opened: the ids the pump needs to keep it attached, plus the
 *  item id the optimistic bubble is relabeled to. */
export type OpenedRun = { runId: RunId; segmentId: SegmentId; userItemId?: string };

interface RunOpeningControllerOptions {
  sessionId: string;
  isCancelled: () => boolean;
  markInteracted: () => void;
  setAbortController: (controller: AbortController) => void;
  abortCurrent: () => void;
  pump: (stream: StreamingResult<OpenedRun, RunEvent>, signal: AbortSignal) => Promise<void>;
  setStartError: (error: AgentProblem) => void;
}

export interface RunOpeningController {
  isStarting: () => boolean;
  begin: (
    run: (signal: AbortSignal) => Promise<StreamingResult<OpenedRun, RunEvent>>,
    onResult?: (result: OpenedRun) => void,
    onStartError?: () => void,
  ) => void;
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

  return {
    isStarting: () => starting,
    begin(run, onResult, onStartError) {
      starting = true;
      const beginId = ++beginSeq;
      markInteracted();
      abortCurrent();
      const ctrl = new AbortController();
      setAbortController(ctrl);
      const span = startRunSpan({ "lyra.session_id": sessionId });
      let failure: unknown;
      let opening: Promise<StreamingResult<OpenedRun, RunEvent>>;
      try {
        opening = withSpan(span, () => run(ctrl.signal));
      } catch (err) {
        opening = Promise.reject(err);
      }
      void opening
        .then((stream) => {
          if (isCancelled() || ctrl.signal.aborted || beginId !== beginSeq) return;
          onResult?.(stream.result);
          span.setAttribute("lyra.run_id", stream.result.runId);
          return pump(stream, ctrl.signal);
        })
        .catch((err: unknown) => {
          if (isCancelled() || ctrl.signal.aborted || beginId !== beginSeq) return;
          failure = err;
          console.error("[agent] run failed to start:", sessionId, err);
          if (err instanceof RpcError)
            // message stays the runtime's own note about this occurrence; the
            // banner turns `code` into words when there wasn't one.
            setStartError({ message: errorDetail(err.data), code: errorType(err.data) });
          onStartError?.();
        })
        .finally(() => {
          if (beginId === beginSeq) starting = false;
          endSpan(span, failure);
        });
    },
  };
}
