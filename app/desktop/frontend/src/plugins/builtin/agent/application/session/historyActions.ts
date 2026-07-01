import type { Message } from "@/plugins/builtin/agent/public/viewState";
import type { AgentInput } from "../../domain/input";
import { notifyInfo } from "@/lib/notify";
import { agentRuntime, type AgentRestoreType } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { agentViewState } from "../ports/viewState";
import { forkSessionAt } from "./forkSession";
import { rehydrateSessionView } from "./rehydrateSession";

export type RestoreType = AgentRestoreType;

export interface ActiveAgentConversation {
  sessionId: string;
  messages: Message[];
}

export function activeAgentConversation(): ActiveAgentConversation | null {
  const sessionId = agentSessionState().getActiveSessionId();
  if (!sessionId) return null;
  return { sessionId, messages: agentViewState().getCurrentView().messages };
}

export function sendToAgentSession(sessionId: string, input: AgentInput): boolean {
  return agentViewState().sendToSession(sessionId, input);
}

export async function rollbackSessionToBeforeRun(
  sessionId: string,
  runId: string,
  restoreType: RestoreType = "history",
): Promise<boolean> {
  const { runs } = await agentRuntime().loadSessionHistory(sessionId);
  const roots = runs.filter((run) => !run.parentRunId && !run.spawnedByItemId);
  const index = roots.findIndex((run) => run.id === runId);
  if (index < 0) return false;
  const keep = index > 0 ? roots[index - 1]!.id : undefined;
  const wantsFiles = restoreType !== "history";
  if (wantsFiles && !keep) {
    notifyInfo("No checkpoint before the first turn — files left as they are.", {
      source: "session",
    });
  }
  await agentRuntime().rollbackSession({
    sessionId,
    ...(keep ? { toRunId: keep } : {}),
    ...(wantsFiles && keep ? { restoreType } : {}),
  });
  await rehydrateSessionView(sessionId);
  return true;
}

export function forkAgentSessionAtRun(sessionId: string, runId: string): Promise<void> {
  return forkSessionAt(sessionId, runId);
}
