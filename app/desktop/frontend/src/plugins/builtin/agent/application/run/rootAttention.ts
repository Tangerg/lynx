import type { AgentRunView, AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView, type AgentSessionViewEntry } from "../ports/sessionView";
import type { AgentRootAttention } from "../view/runTree";
import {
  selectCurrentRootAttention,
  selectCurrentRootRun,
  selectRunProblem,
} from "../view/runTree";
import { isAgentRunFailure } from "../view/runOutcome";

export interface RootRunSettlement {
  sessionId: string;
  status: "needsInput" | "finished" | "error" | "canceled" | "limit";
  errorMessage: string | null;
}

function currentRootAttention(view: AgentSessionView): AgentRootAttention {
  return selectCurrentRootAttention(view);
}

function currentRootRunning(view: AgentSessionView): boolean {
  return currentRootAttention(view).status === "running";
}

function anySessionRunning(sessions: Record<string, AgentSessionViewEntry>): boolean {
  for (const id in sessions) {
    if (currentRootRunning(sessions[id]!.view)) return true;
  }
  return false;
}

function terminalSettlementStatus(
  outcome: AgentRunView["outcome"],
): Exclude<RootRunSettlement["status"], "needsInput"> {
  if (isAgentRunFailure(outcome)) return "error";
  switch (outcome?.type) {
    case "canceled":
      return "canceled";
    case "maxSteps":
    case "maxBudget":
      return "limit";
    default:
      return "finished";
  }
}

export function subscribeAnySessionRunning(onChange: (running: boolean) => void): () => void {
  let lastRunning = anySessionRunning(agentSessionView().getSessions());
  onChange(lastRunning);
  return agentSessionView().subscribeSessions((sessions) => {
    const running = anySessionRunning(sessions);
    if (running === lastRunning) return;
    lastRunning = running;
    onChange(running);
  });
}

export function subscribeRootRunSettlements(
  onSettled: (settlement: RootRunSettlement) => void,
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
