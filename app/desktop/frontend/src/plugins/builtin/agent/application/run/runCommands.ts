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

/** Cancel a root or descendant in the active session. The command is accepted
 * only while that exact Run is non-terminal and a mounted driver owns it. */
export function cancelActiveSessionRun(runId: string): boolean {
  const sessionId = agentSessionState().getActiveSessionId();
  const entry = agentSessionView().getSession(sessionId);
  const run = entry?.view.runsById[runId];
  if (!entry?.cancelRun || !run || run.status === "finished") return false;
  entry.cancelRun(runId);
  return true;
}

export function dismissActiveSessionProblem(): void {
  const sessionId = agentSessionState().getActiveSessionId();
  agentSessionView().clearProblem(sessionId);
}
