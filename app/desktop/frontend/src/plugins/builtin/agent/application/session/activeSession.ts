import type { AgentSessionSummary } from "@/lib/data/queries";
import { useSessions } from "@/lib/data/queries";
import {
  agentSessionState,
  type AgentSessionLifecycleSnapshot,
  type AgentSessionSelectionSnapshot,
} from "../ports/sessionState";

export type {
  AgentSessionLifecycleSnapshot,
  AgentSessionSelectionSnapshot,
} from "../ports/sessionState";

export function useActiveSessionId(): string {
  return agentSessionState().useActiveSessionId();
}

export function getActiveSessionId(): string {
  return agentSessionState().getActiveSessionId();
}

export function getAgentSessionLifecycleSnapshot(): AgentSessionLifecycleSnapshot {
  return agentSessionState().getLifecycleSnapshot();
}

export function subscribeActiveSessionId(onChange: (sessionId: string) => void): () => void {
  return agentSessionState().subscribeActiveSessionId(onChange);
}

export function subscribeAgentSessionLifecycle(
  onChange: (snapshot: AgentSessionLifecycleSnapshot) => void,
): () => void {
  return agentSessionState().subscribeLifecycle(onChange);
}

export function subscribeAgentSessionSelection(
  onChange: (
    snapshot: AgentSessionSelectionSnapshot,
    previous: AgentSessionSelectionSnapshot,
  ) => void,
): () => void {
  return agentSessionState().subscribeSelection(onChange);
}

export function selectAgentSession(id: string): void {
  agentSessionState().selectSession(id);
}

export function closeActiveAgentSession(): boolean {
  const id = getActiveSessionId();
  if (!id) return false;
  agentSessionState().closeSession(id);
  return true;
}

/**
 * The active session summary, or undefined while unknown (no active session /
 * sessions list not loaded yet). The one place the activeSessionId ⨝
 * sessions-cache join lives — chips, banners, and workspace reads all derive
 * from this instead of re-writing the find.
 */
export function useActiveSession(): AgentSessionSummary | undefined {
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
