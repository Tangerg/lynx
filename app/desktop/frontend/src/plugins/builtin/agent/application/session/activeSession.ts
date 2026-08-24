import { discardAbandonedDraft } from "./discardAbandonedDraft";
import type { AgentSessionSummary } from "./sessionQueries";
import { useAgentSessions } from "./sessionQueries";
import { agentSessionState, type AgentSessionLifecycleSnapshot } from "../ports/sessionState";

export type { AgentSessionLifecycleSnapshot } from "../ports/sessionState";

export type ActiveSessionWorkspaceSelection =
  { status: "ready"; cwd?: string } | { status: "resolving"; sessionId: string };

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

export function selectAgentSession(id: string): void {
  agentSessionState().selectSession(id);
}

export function closeActiveAgentSession(): boolean {
  const id = getActiveSessionId();
  if (!id) return false;
  // Before the close, not after: closing drops the session from openSessionIds,
  // which prunes the draft mark this reads — the selection subscriber that covers
  // every other way of leaving a draft would then see an ordinary session.
  discardAbandonedDraft(id);
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
  const { data } = useAgentSessions();
  if (!activeSessionId) return undefined;
  return data?.find((s) => s.id === activeSessionId);
}

/**
 * Resolve the workspace identity independently from its eventual transport use.
 *
 * No active session deliberately means the app's default workspace. An active
 * id absent from the current Session projection means something very different:
 * the projection is still catching up (most visibly just after create or on a
 * cold restore). Keeping that state explicit prevents callers from turning
 * "unknown" into the Runtime's default workspace and reading or mutating the
 * wrong project.
 */
export function activeSessionWorkspaceSelection(
  activeSessionId: string,
  sessions: readonly AgentSessionSummary[] | undefined,
): ActiveSessionWorkspaceSelection {
  if (!activeSessionId) return { status: "ready" };
  const session = sessions?.find((candidate) => candidate.id === activeSessionId);
  return session
    ? { status: "ready", cwd: session.workspace.path }
    : { status: "resolving", sessionId: activeSessionId };
}

export function useActiveSessionWorkspace(): ActiveSessionWorkspaceSelection {
  const activeSessionId = useActiveSessionId();
  const { data } = useAgentSessions();
  return activeSessionWorkspaceSelection(activeSessionId, data);
}
