import type { RunEvent, RunId, SegmentId, StreamingResult } from "@/rpc";
import type { AgentProblem } from "@/plugins/sdk/types/agentSessionView";
import { endSpan, startRunSpan, withSpan } from "@/lib/observability/tracing";
import { agentProblemFromRpcError } from "./rpcProblem";

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
      let opening: ReturnType<typeof run>;
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
          const problem = agentProblemFromRpcError(err);
          if (problem) setStartError(problem);
          onStartError?.();
        })
        .finally(() => {
          if (beginId === beginSeq) starting = false;
          endSpan(span, failure);
        });
    },
  };
}
