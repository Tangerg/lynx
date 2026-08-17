import { agentSessionState } from "../ports/sessionState";
import { agentSessionView } from "../ports/sessionView";

export function useStopCurrentRootRun(): (() => boolean) | null {
  return agentSessionView().useAction("stop");
}

export function stopCurrentRootRun(): boolean {
  const sessionId = agentSessionState().getActiveSessionId();
  const stop = agentSessionView().getSession(sessionId)?.stop;
  if (!stop) return false;
  return stop();
}

interface RunCommandTarget {
  readonly sessionId: string;
  readonly runId: string;
}

/** Cancel the exact root or descendant presented by a Session-owned surface.
 * The command is accepted only while that composite target is non-terminal and
 * its mounted Session driver still owns cancellation. */
export function cancelSessionRun({ sessionId, runId }: RunCommandTarget): boolean {
  const entry = agentSessionView().getSession(sessionId);
  const run = entry?.view.runsById[runId];
  if (!entry?.cancelRun || !run || run.sessionId !== sessionId || run.status === "finished") {
    return false;
  }
  entry.cancelRun(runId);
  return true;
}

export function dismissActiveSessionProblem(): void {
  const sessionId = agentSessionState().getActiveSessionId();
  agentSessionView().clearProblem(sessionId);
}
