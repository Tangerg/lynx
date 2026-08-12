import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { t } from "@/lib/i18n";
import type { AgentInput } from "../../domain/input";
import { notifyInfo } from "@/plugins/sdk";
import { agentRuntime, type RestoreType } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { agentSessionView } from "../ports/sessionView";
import { selectRootNarrativeMessages, selectRootRuns } from "../view/runTree";
import { forkSessionAt } from "./forkSession";
import { rehydrateSessionView } from "./rehydrateSession";
import { projectAgentSessionSnapshot } from "./sessionSnapshot";

export type SessionRollbackResult =
  | { status: "committed"; userInput?: AgentInput }
  | { status: "unavailable" }
  | { status: "inFlight" };

// Rollback rewrites the complete Session history/state boundary. Two different
// rollback commands for one Session cannot be meaningfully interleaved, and a
// double-fired action must not truncate twice and then run its follow-up twice.
const rollbackSessions = new Set<string>();

export interface ActiveAgentConversation {
  sessionId: string;
  messages: Message[];
}

export function activeAgentConversation(): ActiveAgentConversation | null {
  const sessionId = agentSessionState().getActiveSessionId();
  if (!sessionId) return null;
  return {
    sessionId,
    messages: selectRootNarrativeMessages(agentSessionView().getCurrentView()),
  };
}

export function sendToAgentSession(sessionId: string, input: AgentInput): boolean {
  return agentSessionView().sendToSession(sessionId, input);
}

export async function rollbackSessionToBeforeRun(
  sessionId: string,
  runId: string,
  restoreType: RestoreType = "history",
): Promise<SessionRollbackResult> {
  if (rollbackSessions.has(sessionId)) return { status: "inFlight" };
  rollbackSessions.add(sessionId);
  try {
    const snapshot = await agentRuntime().loadSessionSnapshot(sessionId);
    if (!snapshot) return { status: "unavailable" };
    const view = projectAgentSessionSnapshot(snapshot);
    const roots = selectRootRuns(view);
    const index = roots.findIndex((run) => run.id === runId);
    if (index < 0) return { status: "unavailable" };
    const keep = index > 0 ? roots[index - 1]!.id : undefined;
    const wantsFiles = restoreType !== "history";
    if (wantsFiles && !keep) {
      notifyInfo(t("session.restore.noCheckpoint"), {
        source: "session",
      });
      // Protocol requires a concrete checkpoint for files/both and forbids
      // silently degrading either intent to history-only. Omitting both fields
      // here would mean "drop all history", the opposite of files-only.
      return { status: "unavailable" };
    }
    const result = await agentRuntime().rollbackSession({
      sessionId,
      ...(keep ? { toRunId: keep } : {}),
      ...(wantsFiles && keep ? { restoreType } : {}),
    });
    await rehydrateSessionView(sessionId);
    const userInput = result.droppedRuns.find((dropped) => dropped.runId === runId)?.userInput;
    return {
      status: "committed",
      ...(userInput ? { userInput } : {}),
    };
  } finally {
    rollbackSessions.delete(sessionId);
  }
}

export function forkAgentSessionAtRun(sessionId: string, runId: string): Promise<void> {
  return forkSessionAt(sessionId, runId);
}
