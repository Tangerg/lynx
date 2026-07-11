import { useCallback } from "react";
import type { AgentSessionSummary } from "./sessionQueries";
import { queryClient } from "@/lib/data/queryClient";
import { invalidateAgentSessions, AGENT_SESSIONS_KEY } from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionState } from "../ports/sessionState";
import { reportSessionError } from "./reportSessionError";

/**
 * Delete a backend session, close its tab (reselecting a neighbour if it was
 * active), and refetch the session summaries so the row drops. Counterpart to
 * {@link useCreateSession}.
 */
export function useDeleteSession(): (id: string) => Promise<void> {
  return useCallback(async (id) => {
    // Optimistic: drop the row immediately; otherwise it lingers until the
    // delete RPC and the list refetch both complete. Cancel any in-flight
    // refetch first so it can't resolve with the (still-present) row and undo
    // the optimistic removal; snapshot after cancelling for rollback.
    await queryClient.cancelQueries({ queryKey: [AGENT_SESSIONS_KEY] });
    const prev = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
    queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (old) =>
      old?.filter((s) => s.id !== id),
    );
    try {
      await agentRuntime().deleteSession(id);
      agentSessionState().closeSession(id);
      void invalidateAgentSessions();
    } catch (err) {
      if (prev) queryClient.setQueryData([AGENT_SESSIONS_KEY], prev);
      reportSessionError("delete", err);
    }
  }, []);
}
