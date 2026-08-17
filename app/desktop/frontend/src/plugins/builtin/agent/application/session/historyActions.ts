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
import { agentCommandOwner } from "../agentCommandOwner";

export type SessionRollbackResult =
  | { status: "committed"; userInput?: AgentInput }
  | { status: "unavailable" }
  | { status: "inFlight" };

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
  const owner = agentCommandOwner();
  const lease = owner.beginSessionRollback(sessionId);
  if (!lease) return { status: "inFlight" };
  // One captured gateway owns both the pre-command inspection and the write. A
  // replacement between them retires the lease instead of splicing two clients.
  const runtime = agentRuntime();
  try {
    const material = await owner.settle(runtime.loadSessionSnapshot(sessionId));
    owner.assertCurrent();
    if (!material) return { status: "unavailable" };
    // This is a pre-command inspection, not a mounted projection commit. Its
    // associated read models must not replace what the UI currently owns.
    const view = projectAgentSessionSnapshot(material.snapshot);
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
    const result = await owner.settle(
      runtime.rollbackSession({
        sessionId,
        ...(keep ? { toRunId: keep } : {}),
        ...(wantsFiles && keep ? { restoreType } : {}),
      }),
    );
    owner.assertCurrent();
    await owner.settle(rehydrateSessionView(sessionId));
    owner.assertCurrent();
    const userInput = result.droppedRuns.find((dropped) => dropped.runId === runId)?.userInput;
    return {
      status: "committed",
      ...(userInput ? { userInput } : {}),
    };
  } catch (error) {
    if (!lease.isCurrent()) return { status: "unavailable" };
    throw error;
  } finally {
    lease.release();
  }
}

export function forkAgentSessionAtRun(sessionId: string, runId: string): Promise<void> {
  return forkSessionAt(sessionId, runId);
}
