import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import type { SessionProjectionSynchronizationOwnership } from "../ports/sessionView";
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

export interface AgentSessionProjectionRevalidation {
  /** Projection built from this read even when a newer local write prevents it
   * from replacing the material view. Command settlement may still use this
   * neutral fact without forcing a stale projection commit. */
  authoritativeView: AgentSessionView;
  committed: boolean;
}

export async function refreshAgentSessionProjection(
  sessionId: string,
  options: RefreshSessionProjectionOptions = {},
): Promise<AgentSessionView | null> {
  const result = await revalidateAgentSessionProjection(sessionId, options);
  return result?.committed ? result.authoritativeView : null;
}

export async function revalidateAgentSessionProjection(
  sessionId: string,
  options: RefreshSessionProjectionOptions = {},
): Promise<AgentSessionProjectionRevalidation | null> {
  const viewPort = agentSessionView();
  const token = viewPort.beginViewRefresh(sessionId, options.invalidateQueuedRunEvents ?? false);
  if (!token) return null;

  const snapshot = await agentRuntime().loadSessionSnapshot(sessionId);
  if (!snapshot || (options.canCommit && !options.canCommit())) return null;
  const view = projectAgentSessionSnapshot(snapshot);
  return {
    authoritativeView: view,
    committed: viewPort.commitViewRefresh(sessionId, token, view),
  };
}

export interface MountedAgentSessionSynchronization {
  sessionIds?: readonly string[];
  /** A global reconciliation boundary supersedes any live stream from the
   * previous Runtime/event generation before reading durable truth. */
  ownership?: SessionProjectionSynchronizationOwnership;
}

export function synchronizeMountedAgentSessions(
  request: MountedAgentSessionSynchronization = {},
): void {
  const sessions = agentSessionView().getSessions();
  const mountedIds = Object.keys(sessions);
  const requested = request.sessionIds ? new Set(request.sessionIds) : null;
  const targets = requested
    ? mountedIds.filter((sessionId) => requested.has(sessionId))
    : mountedIds;
  for (const sessionId of targets) {
    const synchronize = sessions[sessionId]?.synchronize;
    if (synchronize) void synchronize(request.ownership);
    else void refreshAgentSessionProjection(sessionId);
  }
}
