import type { RunEvent, RunId, StreamingResult } from "@/rpc";
import { errorDetail, errorType, RpcError } from "@/rpc";
import type { RunError } from "@/plugins/builtin/agent/public/viewState";
import { endSpan, startRunSpan, withSpan } from "@/lib/observability/tracing";

interface RunOpeningControllerOptions {
  sessionId: string;
  isCancelled: () => boolean;
  markInteracted: () => void;
  setAbortController: (controller: AbortController) => void;
  abortCurrent: () => void;
  pump: (
    stream: StreamingResult<{ runId: RunId; userItemId?: string }, RunEvent>,
    signal: AbortSignal,
  ) => Promise<void>;
  setStartError: (error: RunError) => void;
}

export interface RunOpeningController {
  isStarting: () => boolean;
  begin: (
    run: (
      signal: AbortSignal,
    ) => Promise<StreamingResult<{ runId: RunId; userItemId?: string }, RunEvent>>,
    onResult?: (result: { runId: RunId; userItemId?: string }) => void,
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
      let opening: Promise<StreamingResult<{ runId: RunId; userItemId?: string }, RunEvent>>;
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
            setStartError({
              message: errorDetail(err.data) ?? err.message,
              code: errorType(err.data),
            });
          onStartError?.();
        })
        .finally(() => {
          if (beginId === beginSeq) starting = false;
          endSpan(span, failure);
        });
    },
  };
}
