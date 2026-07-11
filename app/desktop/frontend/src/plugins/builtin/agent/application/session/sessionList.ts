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
  const done = useRef(false);
  useEffect(() => {
    if (done.current || !isSuccess) return;
    done.current = true;
    agentSessionState().reconcileSessions((data ?? []).map((session) => session.id));
  }, [isSuccess, data]);
}
