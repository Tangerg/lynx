import type { SidebarSession } from "@/lib/data/queries";
import { useEffect, useMemo, useRef } from "react";
import { useSessions } from "@/lib/data/queries";
import { useAgentSessionStore } from "@/state/agentSessionStore";

const EMPTY_SESSIONS: SidebarSession[] = [];

export function useVisibleAgentSessions(): SidebarSession[] {
  const { data } = useSessions();
  const draftIds = useAgentSessionStore((state) => state.draftSessionIds);
  const sessions = data ?? EMPTY_SESSIONS;
  return useMemo(
    () => sessions.filter((session) => !draftIds.has(session.id)),
    [sessions, draftIds],
  );
}

export function useSelectAgentSession(): (id: string) => void {
  return useAgentSessionStore((state) => state.selectTab);
}

export function useReconcilePersistedAgentSessions(): void {
  const { data, isSuccess } = useSessions();
  const done = useRef(false);
  useEffect(() => {
    if (done.current || !isSuccess) return;
    done.current = true;
    useAgentSessionStore.getState().reconcileTabs((data ?? []).map((session) => session.id));
  }, [isSuccess, data]);
}
