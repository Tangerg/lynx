import { useCallback } from "react";
import type { AgentSessionSummary } from "./sessionQueries";
import { queryClient } from "@/lib/queryClient";
import {
  invalidateAgentSessions,
  recoverAgentSessionSummaryField,
  AGENT_SESSIONS_KEY,
} from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { reportSessionError } from "./reportSessionError";
import { agentCommandOwner, type AgentCommandEffect } from "../agentCommandOwner";

/** Pin / unpin a session (sessions.update favorite) and refresh session summaries.
 *  Optimistic: flips the star in the list right away so the row reorders
 *  without waiting for the RPC + refetch; rolls back on failure. */
export function useToggleFavorite(): (
  id: string,
  expectedRevision: number,
  favorite: boolean,
) => Promise<void> {
  return useCallback(async (id, expectedRevision, favorite) => {
    const owner = agentCommandOwner();
    const runtime = agentRuntime();
    let effect: AgentCommandEffect | undefined;
    // Cancel any in-flight sessions refetch before the optimistic write so a
    // background invalidate (workspace resync / reconnect) started earlier
    // can't resolve with the old favorite flag and un-flip the star.
    try {
      await owner.settle(queryClient.cancelQueries({ queryKey: [AGENT_SESSIONS_KEY] }));
      owner.assertCurrent();
      const prev = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
      queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (old) =>
        old?.map((s) => (s.id === id ? { ...s, favorite } : s)),
      );
      effect = owner.trackEffect(() =>
        recoverAgentSessionSummaryField(prev, id, "favorite", favorite),
      );
      const updated = await owner.settleSessionSummary(id, expectedRevision, (revision) =>
        runtime.updateSession({
          sessionId: id,
          expectedRevision: revision,
          favorite,
        }),
      );
      owner.assertCurrent();
      queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (old) =>
        old?.map((s) => (s.id === id ? { ...s, revision: updated.revision } : s)),
      );
      effect.settle();
      void invalidateAgentSessions();
    } catch (err) {
      if (!owner.isCurrent()) return;
      effect?.rollback();
      reportSessionError("favorite", err);
    }
  }, []);
}
