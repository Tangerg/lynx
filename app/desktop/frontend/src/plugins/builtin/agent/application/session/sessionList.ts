import type { AgentSessionSummary } from "@/lib/data/queries";
import { useEffect, useMemo, useRef } from "react";
import { useSessions } from "@/lib/data/queries";
import { agentSessionState } from "../ports/sessionState";

const EMPTY_SESSIONS: AgentSessionSummary[] = [];

export function useVisibleAgentSessions(): AgentSessionSummary[] {
  const { data } = useSessions();
  const draftIds = agentSessionState().useDraftSessionIds();
  const sessions = data ?? EMPTY_SESSIONS;
  return useMemo(
    () => sessions.filter((session) => !draftIds.has(session.id)),
    [sessions, draftIds],
  );
}

export function useReconcilePersistedAgentSessions(): void {
  const { data, isSuccess } = useSessions();
  const done = useRef(false);
  useEffect(() => {
    if (done.current || !isSuccess) return;
    done.current = true;
    agentSessionState().reconcileSessions((data ?? []).map((session) => session.id));
  }, [isSuccess, data]);
}
