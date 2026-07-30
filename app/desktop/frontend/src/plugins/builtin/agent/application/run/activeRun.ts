import type {
  AgentProblem,
  AgentRunView,
  AgentSessionView,
  PlanItem,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { agentSessionState } from "../ports/sessionState";
import { agentSessionView, type AgentSessionViewEntry } from "../ports/sessionView";
import type { AgentRootAttention, AgentRunTreeNode } from "../view/runTree";
import {
  selectCurrentRootAttention,
  selectCurrentRootRun,
  selectRunProblem,
} from "../view/runTree";

export interface AgentRunSettlement {
  sessionId: string;
  status: "needsInput" | "finished" | "error" | "canceled" | "limit";
  errorMessage: string | null;
}

export function useIsAgentRunning(): boolean {
  return agentSessionView().useCurrentRootAttention().status === "running";
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

export function useActiveRunTree(): AgentRunTreeNode[] {
  return agentSessionView().useRunTree();
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

function currentRootAttention(view: AgentSessionView): AgentRootAttention {
  return selectCurrentRootAttention(view);
}

function currentRootRunning(view: AgentSessionView): boolean {
  return currentRootAttention(view).status === "running";
}

function anyAgentRunning(sessions: Record<string, AgentSessionViewEntry>): boolean {
  for (const id in sessions) {
    if (currentRootRunning(sessions[id]!.view)) return true;
  }
  return false;
}

function terminalSettlementStatus(
  outcome: AgentRunView["outcome"],
): Exclude<AgentRunSettlement["status"], "needsInput"> {
  switch (outcome?.type) {
    case "error":
      return "error";
    case "canceled":
      return "canceled";
    case "maxSteps":
    case "maxBudget":
      return "limit";
    default:
      return "finished";
  }
}

export function subscribeAnyAgentRunning(onChange: (running: boolean) => void): () => void {
  let lastRunning = anyAgentRunning(agentSessionView().getSessions());
  onChange(lastRunning);
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
  const previousBySession = new Map<string, AgentRootAttention>();
  for (const [sessionId, entry] of Object.entries(agentSessionView().getSessions())) {
    previousBySession.set(sessionId, currentRootAttention(entry.view));
  }
  return agentSessionView().subscribeSessions((sessions) => {
    for (const sessionId in sessions) {
      const view = sessions[sessionId]!.view;
      const current = currentRootAttention(view);
      const previous = previousBySession.get(sessionId) ?? { status: "idle", runId: null };
      if (current.runId === previous.runId && current.status === previous.status) continue;
      previousBySession.set(sessionId, current);

      const sameRunSettled =
        previous.runId !== null &&
        previous.runId === current.runId &&
        (previous.status === "running" || previous.status === "waiting") &&
        (current.status === "waiting" || current.status === "finished");
      if (sameRunSettled) {
        const root = selectCurrentRootRun(view);
        onSettled({
          sessionId,
          status:
            current.status === "waiting"
              ? "needsInput"
              : terminalSettlementStatus(root?.outcome ?? null),
          errorMessage:
            current.status === "finished" ? (selectRunProblem(root)?.message ?? null) : null,
        });
      }
    }
    for (const sessionId of [...previousBySession.keys()]) {
      if (!(sessionId in sessions)) previousBySession.delete(sessionId);
    }
  });
}
