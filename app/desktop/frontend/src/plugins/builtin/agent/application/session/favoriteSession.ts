import { useCallback } from "react";
import type { AgentSessionSummary } from "./sessionQueries";
import { queryClient } from "@/lib/data/queryClient";
import { invalidateAgentSessions, AGENT_SESSIONS_KEY } from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { reportSessionError } from "./reportSessionError";

/** Pin / unpin a session (sessions.update favorite) and refresh session summaries.
 *  Optimistic: flips the star in the list right away so the row reorders
 *  without waiting for the RPC + refetch; rolls back on failure. */
export function useToggleFavorite(): (id: string, favorite: boolean) => Promise<void> {
  return useCallback(async (id, favorite) => {
    // Cancel any in-flight sessions refetch before the optimistic write so a
    // background invalidate (workspace resync / reconnect) started earlier
    // can't resolve with the old favorite flag and un-flip the star.
    await queryClient.cancelQueries({ queryKey: [AGENT_SESSIONS_KEY] });
    const prev = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
    queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (old) =>
      old?.map((s) => (s.id === id ? { ...s, favorite } : s)),
    );
    try {
      await agentRuntime().updateSession({ sessionId: id, favorite });
      void invalidateAgentSessions();
    } catch (err) {
      if (prev) queryClient.setQueryData([AGENT_SESSIONS_KEY], prev);
      reportSessionError("favorite", err);
    }
  }, []);
}
