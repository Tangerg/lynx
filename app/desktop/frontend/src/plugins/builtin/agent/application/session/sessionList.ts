import type { AgentSessionSummary } from "./sessionQueries";
import { useEffect, useMemo, useRef } from "react";
import { useAgentSessions } from "./sessionQueries";
import { agentSessionState } from "../ports/sessionState";

const EMPTY_SESSIONS: AgentSessionSummary[] = [];

export function useVisibleAgentSessions(): AgentSessionSummary[] {
  const { data } = useAgentSessions();
  const draftIds = agentSessionState().useDraftSessionIds();
  const sessions = data ?? EMPTY_SESSIONS;
  return useMemo(
    () => sessions.filter((session) => !draftIds.has(session.id)),
    [sessions, draftIds],
  );
}

export function useReconcilePersistedAgentSessions(): void {
  const { data, isSuccess } = useAgentSessions();
  const restored = useRef(false);
  useEffect(() => {
    if (!isSuccess) return;
    const sessions = data ?? [];
    if (!restored.current) {
      restored.current = true;
      // Seed the location from memory before the first reconciliation: a cold
      // start always opens at "/" with no session, and where the user was is
      // remembered rather than owned (see lib/navigation). Later authoritative
      // reads must reconcile deletion without replaying this boot-only move.
      agentSessionState().restoreLastSession();
    }
    // Reconcile every successful Runtime read: sessions.changed can remove an
    // active Session from another client long after boot.
    // Empty sessions that belong to another client remain visible. This client
    // cannot infer their draft ownership after a cold start, so only the
    // owner-scoped navigation cleanup may delete an unused draft.
    agentSessionState().reconcileSessions(sessions.map((session) => session.id));
  }, [isSuccess, data]);
}
