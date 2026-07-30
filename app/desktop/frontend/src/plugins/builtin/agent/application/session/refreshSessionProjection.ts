import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import { agentRuntime } from "../ports/runtimeGateway";
import { agentSessionView } from "../ports/sessionView";
import { projectAgentSessionSnapshot } from "./sessionSnapshot";

interface RefreshSessionProjectionOptions {
  /** A server-side history rewrite invalidates queued events from the replaced history.
   *  Ordinary runtime invalidation keeps the live stream generation intact. */
  invalidateQueuedRunEvents?: boolean;
  /** Cold-open recovery uses this to reject a fetch after local interaction or
   *  teardown without coupling this use case to React lifecycle state. */
  canCommit?: () => boolean;
}

export async function refreshAgentSessionProjection(
  sessionId: string,
  options: RefreshSessionProjectionOptions = {},
): Promise<AgentSessionView | null> {
  const viewPort = agentSessionView();
  const token = viewPort.beginViewRefresh(sessionId, options.invalidateQueuedRunEvents ?? false);
  if (!token) return null;

  const snapshot = await agentRuntime().loadSessionSnapshot(sessionId);
  if (options.canCommit && !options.canCommit()) return null;
  const view = projectAgentSessionSnapshot(snapshot);
  return viewPort.commitViewRefresh(sessionId, token, view) ? view : null;
}

export function synchronizeMountedAgentSessions(sessionIds?: readonly string[]): void {
  const sessions = agentSessionView().getSessions();
  const mountedIds = Object.keys(sessions);
  const requested = sessionIds ? new Set(sessionIds) : null;
  const targets = requested
    ? mountedIds.filter((sessionId) => requested.has(sessionId))
    : mountedIds;
  for (const sessionId of targets) {
    const synchronize = sessions[sessionId]?.synchronize;
    if (synchronize) synchronize();
    else void refreshAgentSessionProjection(sessionId);
  }
}
