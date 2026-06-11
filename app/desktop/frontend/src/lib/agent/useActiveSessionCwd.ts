import { useSessions } from "@/lib/data/queries";
import { useSessionStore } from "@/state/sessionStore";

/**
 * The active session's working directory, or undefined while unknown (no
 * active session / sessions list not loaded yet). Workspace reads (VCS
 * panels, grep / file-head previews) pass this as `cwd` so they reflect
 * the session's project rather than the serve-process directory — the
 * runtime may serve from anywhere (BACKEND_API_REFERENCE §5 registers
 * watches with an explicit project cwd for the same reason).
 */
export function useActiveSessionCwd(): string | undefined {
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const { data } = useSessions();
  if (!activeSessionId) return undefined;
  return data?.find((s) => s.id === activeSessionId)?.cwd;
}
