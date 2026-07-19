import { useCallback } from "react";
import type { AgentSessionSummary } from "./sessionQueries";
import { queryClient } from "@/lib/data/queryClient";
import { invalidateAgentSessions, AGENT_SESSIONS_KEY } from "./sessionQueries";
import { agentRuntime } from "../ports/runtimeGateway";
import { reportSessionError } from "./reportSessionError";

/** Rename a session (sessions.update title) and refresh session summaries.
 *  Empty titles are rejected server-side (invalid_params) — callers trim
 *  and skip no-op submissions before getting here. */
export function useRenameSession(): (
  id: string,
  expectedRevision: number,
  title: string,
) => Promise<void> {
  return useCallback(async (id, expectedRevision, title) => {
    // Optimistic: paint the new title in the session summary list right away so the
    // row doesn't flash back to the old title while the RPC + refetch settle.
    // Cancel any in-flight sessions refetch FIRST: one started before this
    // optimistic write (a background workspace resync / reconnect invalidate)
    // would otherwise resolve with pre-rename data and clobber the optimistic
    // title. Snapshot after cancelling so rollback restores the right state.
    await queryClient.cancelQueries({ queryKey: [AGENT_SESSIONS_KEY] });
    const prev = queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]);
    queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (old) =>
      old?.map((s) => (s.id === id ? { ...s, title } : s)),
    );
    try {
      const updated = await agentRuntime().updateSession({
        sessionId: id,
        expectedRevision,
        title,
      });
      queryClient.setQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY], (old) =>
        old?.map((s) => (s.id === id ? { ...s, revision: updated.revision } : s)),
      );
      void invalidateAgentSessions();
    } catch (err) {
      if (prev) queryClient.setQueryData([AGENT_SESSIONS_KEY], prev);
      reportSessionError("rename", err);
    }
  }, []);
}
