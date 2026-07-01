import type { SidebarSession } from "@/lib/data/queries";
import { useSessions } from "@/lib/data/queries";
import { useAgentSessionStore } from "@/state/agentSessionStore";

export interface AgentSessionLifecycleSnapshot {
  activeSessionId: string;
  openSessionIds: string[];
}

export function useActiveSessionId(): string {
  return useAgentSessionStore((s) => s.activeSessionId);
}

export function getActiveSessionId(): string {
  return useAgentSessionStore.getState().activeSessionId;
}

export function getAgentSessionLifecycleSnapshot(): AgentSessionLifecycleSnapshot {
  const state = useAgentSessionStore.getState();
  return { activeSessionId: state.activeSessionId, openSessionIds: state.tabIds };
}

export function subscribeActiveSessionId(onChange: (sessionId: string) => void): () => void {
  let lastSessionId = getActiveSessionId();
  return useAgentSessionStore.subscribe((state) => {
    if (state.activeSessionId === lastSessionId) return;
    lastSessionId = state.activeSessionId;
    onChange(lastSessionId);
  });
}

export function subscribeAgentSessionLifecycle(
  onChange: (snapshot: AgentSessionLifecycleSnapshot) => void,
): () => void {
  let lastSnapshot = getAgentSessionLifecycleSnapshot();
  return useAgentSessionStore.subscribe((state) => {
    if (
      state.activeSessionId === lastSnapshot.activeSessionId &&
      state.tabIds === lastSnapshot.openSessionIds
    ) {
      return;
    }
    lastSnapshot = { activeSessionId: state.activeSessionId, openSessionIds: state.tabIds };
    onChange(lastSnapshot);
  });
}

export function selectAgentSession(id: string): void {
  useAgentSessionStore.getState().selectTab(id);
}

export function closeActiveAgentSession(): boolean {
  const id = getActiveSessionId();
  if (!id) return false;
  useAgentSessionStore.getState().closeTab(id);
  return true;
}

/**
 * The active session's sidebar row, or undefined while unknown (no active
 * session / sessions list not loaded yet). The one place the
 * activeSessionId ⨝ sessions-cache join lives — chips, banners, and
 * workspace reads all derive from this instead of re-writing the find.
 */
export function useActiveSession(): SidebarSession | undefined {
  const activeSessionId = useActiveSessionId();
  const { data } = useSessions();
  if (!activeSessionId) return undefined;
  return data?.find((s) => s.id === activeSessionId);
}

/**
 * The active session's working directory. Workspace reads (VCS panels,
 * grep / file-head previews, memory) pass this as `cwd` so they reflect
 * the session's project rather than the serve-process directory — the
 * runtime may serve from anywhere (BACKEND_API_REFERENCE §5 registers
 * watches with an explicit project cwd for the same reason).
 */
export function useActiveSessionCwd(): string | undefined {
  return useActiveSession()?.cwd;
}
