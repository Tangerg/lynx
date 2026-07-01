import type { SidebarSession } from "@/lib/data/queries";
import { useSessions } from "@/lib/data/queries";
import { useSessionStore } from "@/state/sessionStore";

export function useActiveSessionId(): string {
  return useSessionStore((s) => s.activeSessionId);
}

export function getActiveSessionId(): string {
  return useSessionStore.getState().activeSessionId;
}

export function selectAgentSession(id: string): void {
  useSessionStore.getState().selectTab(id);
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
