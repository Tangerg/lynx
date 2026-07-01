import type { Message } from "@/plugins/builtin/agent/public/viewState";
import type { AgentInput } from "../../domain/input";
import { getContainer } from "@/main/container";
import { notifyInfo } from "@/lib/notify";
import { asRunId, asSessionId } from "@/rpc";
import { agentSessionState } from "../ports/sessionState";
import { agentViewState } from "../ports/viewState";
import { forkSessionAt } from "./forkSession";
import { rehydrateSessionView } from "./rehydrateSession";

export type RestoreType = "history" | "files" | "both";

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
  const client = getContainer().client();
  const sid = asSessionId(sessionId);
  const { runs } = await client.items.list({ sessionId: sid });
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
  await client.sessions.rollback({
    sessionId: sid,
    ...(keep ? { toRunId: asRunId(keep) } : {}),
    ...(wantsFiles && keep ? { restoreType } : {}),
  });
  await rehydrateSessionView(sessionId);
  return true;
}

export function forkAgentSessionAtRun(sessionId: string, runId: string): Promise<void> {
  return forkSessionAt(sessionId, asRunId(runId));
}
