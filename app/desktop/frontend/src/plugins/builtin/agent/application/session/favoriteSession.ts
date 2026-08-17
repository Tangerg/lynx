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
import { agentCommandOwner } from "../agentCommandOwner";

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
    // Cancel any in-flight sessions refetch before the optimistic write so a
    // background invalidate (workspace resync / reconnect) started earlier
    // can't resolve with the old favorite flag and un-flip the star.
    await queryClient.cancelQueries({ queryKey: [AGENT_SESSIONS_KEY] });
    if (!owner.isCurrent()) return;
    const prev = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
    queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (old) =>
      old?.map((s) => (s.id === id ? { ...s, favorite } : s)),
    );
    const effect = owner.trackEffect(() =>
      recoverAgentSessionSummaryField(prev, id, "favorite", favorite),
    );
    try {
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
      effect.rollback();
      reportSessionError("favorite", err);
    }
  }, []);
}
