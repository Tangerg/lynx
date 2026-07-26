import type { AgentSessionSummary } from "./sessionQueries";
import { useEffect, useMemo, useRef } from "react";
import { useAgentSessions } from "./sessionQueries";
import { agentSessionState } from "../ports/sessionState";
import { pruneUnusedSessions } from "./pruneUnusedSessions";

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
  const done = useRef(false);
  useEffect(() => {
    if (done.current || !isSuccess) return;
    done.current = true;
    const sessions = data ?? [];
    // Reconcile FIRST: it decides which sessions the app still holds open, and the
    // sweep must not delete the one being restored into view.
    agentSessionState().reconcileSessions(sessions.map((session) => session.id));
    void pruneUnusedSessions(sessions, agentSessionState().getLifecycleSnapshot().openSessionIds);
  }, [isSuccess, data]);
}
