import type {
  AgentProblem,
  AgentSessionView,
  PlanItem,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { agentSessionState } from "../ports/sessionState";
import { agentSessionView, type AgentSessionViewEntry } from "../ports/sessionView";
import { selectCurrentRootRun, selectVisibleProblem } from "../view/runTree";

interface AgentRunSettlement {
  sessionId: string;
  needsInput: boolean;
  errorMessage: string | null;
}

export function useIsAgentRunning(): boolean {
  return agentSessionView().useCurrentRootRunning();
}

export function useActiveRunId(): string | null {
  return agentSessionView().useCurrentRootRunId();
}

export function useActiveRunPlan(): PlanItem[] {
  return agentSessionView().useCurrentRootPlan();
}

export function useActiveRunToolCalls(): Record<string, ToolCall> {
  return agentSessionView().useToolCalls();
}

export function useActiveRunTimeline(): TimelineEntry[] {
  return agentSessionView().useSessionTimeline();
}

export function useVisibleAgentProblem(): AgentProblem | null {
  return agentSessionView().useProblem();
}

export function useStopActiveAgentRun(): (() => void) | null {
  return agentSessionView().useAction("stop");
}

export function stopActiveAgentRun(): boolean {
  const sessionId = agentSessionState().getActiveSessionId();
  const entry = agentSessionView().getSession(sessionId);
  const runId = entry ? selectCurrentRootRun(entry.view)?.id : null;
  return runId ? cancelAgentRun(runId) : false;
}

/** Cancel a root or descendant in the active session. The command is accepted
 * only while that exact Run is non-terminal and a mounted driver owns it. */
export function cancelAgentRun(runId: string): boolean {
  const sessionId = agentSessionState().getActiveSessionId();
  const entry = agentSessionView().getSession(sessionId);
  const run = entry?.view.runsById[runId];
  if (!entry?.cancelRun || !run || run.status === "finished") return false;
  entry.cancelRun(runId);
  return true;
}

export function dismissVisibleAgentProblem(): void {
  const sessionId = agentSessionState().getActiveSessionId();
  if (!sessionId) return;
  agentSessionView().clearProblem(sessionId);
}

function currentRootRunning(view: AgentSessionView): boolean {
  return selectCurrentRootRun(view)?.status === "running";
}

function anyAgentRunning(sessions: Record<string, AgentSessionViewEntry>): boolean {
  for (const id in sessions) {
    if (currentRootRunning(sessions[id]!.view)) return true;
  }
  return false;
}

export function subscribeAnyAgentRunning(onChange: (running: boolean) => void): () => void {
  let lastRunning = anyAgentRunning(agentSessionView().getSessions());
  return agentSessionView().subscribeSessions((sessions) => {
    const running = anyAgentRunning(sessions);
    if (running === lastRunning) return;
    lastRunning = running;
    onChange(running);
  });
}

export function subscribeAgentRunSettlements(
  onSettled: (settlement: AgentRunSettlement) => void,
): () => void {
  const lastRunning = new Map<string, boolean>();
  return agentSessionView().subscribeSessions((sessions) => {
    let count = 0;
    for (const sessionId in sessions) {
      count++;
      const view = sessions[sessionId]!.view;
      const running = currentRootRunning(view);
      const wasRunning = lastRunning.get(sessionId) ?? false;
      if (wasRunning === running) continue;
      lastRunning.set(sessionId, running);
      if (wasRunning && !running) {
        onSettled({
          sessionId,
          needsInput: view.pendingInterrupts.length > 0,
          errorMessage: selectVisibleProblem(view)?.message ?? null,
        });
      }
    }
    if (lastRunning.size > count) {
      for (const sessionId of [...lastRunning.keys()]) {
        if (!(sessionId in sessions)) lastRunning.delete(sessionId);
      }
    }
  });
}
