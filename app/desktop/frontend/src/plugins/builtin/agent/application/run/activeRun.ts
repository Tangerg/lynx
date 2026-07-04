import type {
  PlanItem,
  RunError,
  TimelineEntry,
  ToolCall,
} from "@/plugins/builtin/agent/public/viewState";
import { agentSessionState } from "../ports/sessionState";
import { agentViewState } from "../ports/viewState";

interface AgentSessionEntry {
  view: {
    run: { running: boolean };
    openInterrupts: unknown[];
    error: RunError | null;
  };
}

interface AgentRunSettlement {
  sessionId: string;
  needsInput: boolean;
  errorMessage: string | null;
}

export function useIsAgentRunning(): boolean {
  return agentViewState().useRunning();
}

export function useActiveRunId(): string | null {
  return agentViewState().useRunId();
}

export function useActiveRunPlan(): PlanItem[] {
  return agentViewState().usePlan();
}

export function useActiveRunToolCalls(): Record<string, ToolCall> {
  return agentViewState().useToolCalls();
}

export function useActiveRunTimeline(): TimelineEntry[] {
  return agentViewState().useTimeline();
}

export function useActiveRunError(): RunError | null {
  return agentViewState().useError();
}

export function useStopActiveAgentRun(): (() => void) | null {
  return agentViewState().useAction("stop");
}

export function stopActiveAgentRun(): boolean {
  const sessionId = agentSessionState().getActiveSessionId();
  const entry = agentViewState().getSession(sessionId);
  if (!entry?.view.run.running) return false;
  entry.stop?.();
  return true;
}

export function clearActiveRunError(): void {
  const sessionId = agentSessionState().getActiveSessionId();
  if (!sessionId) return;
  agentViewState().clearError(sessionId);
}

function anyAgentRunning(sessions: Record<string, AgentSessionEntry>): boolean {
  for (const id in sessions) {
    if (sessions[id]!.view.run.running) return true;
  }
  return false;
}

export function subscribeAnyAgentRunning(onChange: (running: boolean) => void): () => void {
  let lastRunning = anyAgentRunning(agentViewState().getSessions());
  return agentViewState().subscribeSessions((sessions) => {
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
  return agentViewState().subscribeSessions((sessions) => {
    let count = 0;
    for (const sessionId in sessions) {
      count++;
      const view = sessions[sessionId]!.view;
      const running = view.run.running;
      const was = lastRunning.get(sessionId) ?? false;
      if (was === running) continue;
      lastRunning.set(sessionId, running);
      if (was && !running) {
        onSettled({
          sessionId,
          needsInput: view.openInterrupts.length > 0,
          errorMessage: view.error?.message ?? null,
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
