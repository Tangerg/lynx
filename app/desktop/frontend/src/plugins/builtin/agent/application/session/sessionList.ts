import type { SidebarSession } from "@/lib/data/queries";
import { useEffect, useMemo, useRef } from "react";
import { useSessions } from "@/lib/data/queries";
import { useSessionStore } from "@/state/sessionStore";

const EMPTY_SESSIONS: SidebarSession[] = [];

export function useVisibleAgentSessions(): SidebarSession[] {
  const { data } = useSessions();
  const draftIds = useSessionStore((state) => state.draftSessionIds);
  const sessions = data ?? EMPTY_SESSIONS;
  return useMemo(
    () => sessions.filter((session) => !draftIds.has(session.id)),
    [sessions, draftIds],
  );
}

export function useSelectAgentSession(): (id: string) => void {
  return useSessionStore((state) => state.selectTab);
}

export function useReconcilePersistedAgentSessions(): void {
  const { data, isSuccess } = useSessions();
  const done = useRef(false);
  useEffect(() => {
    if (done.current || !isSuccess) return;
    done.current = true;
    useSessionStore.getState().reconcileTabs((data ?? []).map((session) => session.id));
  }, [isSuccess, data]);
}
