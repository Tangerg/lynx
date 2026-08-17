import { useCallback } from "react";
import { invalidateAgentSessions } from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner } from "../agentCommandOwner";

/** Imperative fork for non-React callers (message context-menu actions).
 *  `fromRunId` = branch up to AND INCLUDING that root run (AUX_API §4.2);
 *  omitted = whole-session copy. The fork inherits the source's chat history,
 *  so unlike a fresh create it is no draft — it shows in the Work Index
 *  immediately, and we open its tab. */
export function forkSessionAt(id: string, fromRunId?: string): Promise<void> {
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  const state = agentSessionState();
  const key = fromRunId ? `${id}:${fromRunId}` : id;
  return owner
    .runSessionFork(key, async () => {
      const fork = await runtime.forkSession({ sessionId: id, fromRunId });
      owner.assertCurrent();
      state.selectSession(fork.id);
      void invalidateAgentSessions();
    })
    .catch((error: unknown) => {
      if (owner.isCurrent()) reportSessionError("fork", error);
    });
}

export function useForkSession(): (id: string) => Promise<void> {
  // Stable identity for React callers; the imperative core owns the logic.
  return useCallback((id) => forkSessionAt(id), []);
}
