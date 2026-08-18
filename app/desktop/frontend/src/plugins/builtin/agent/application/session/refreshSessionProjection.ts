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
  /** Lifecycle owner for a Runtime generation. An aborted read cannot commit,
   * even when the gateway settles late or ignores cancellation. */
  signal?: AbortSignal;
}

const ABORTED = Symbol("agent-session-snapshot.aborted");

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

  const read = agentRuntime().loadSessionSnapshot(sessionId, options.signal);
  const material = options.signal ? await settleBeforeAbort(read, options.signal) : await read;
  if (material === ABORTED) return null;
  if (!material || (options.canCommit && !options.canCommit())) return null;
  const projected = projectAgentSessionSnapshot(material.snapshot);
  const shared = material.projectAssociatedSharedMaterial(projected.shared);
  const view = shared === projected.shared ? projected : { ...projected, shared };
  const committed = viewPort.commitViewRefresh(sessionId, token, view);
  return {
    authoritativeView: view,
    committed,
  };
}

function settleBeforeAbort<T>(
  operation: Promise<T>,
  signal: AbortSignal,
): Promise<T | typeof ABORTED> {
  if (signal.aborted) {
    void operation.catch(() => undefined);
    return Promise.resolve(ABORTED);
  }
  return new Promise((resolve, reject) => {
    let settled = false;
    const onAbort = () => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", onAbort);
      resolve(ABORTED);
    };
    signal.addEventListener("abort", onAbort, { once: true });
    operation.then(
      (value) => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        resolve(value);
      },
      (error: unknown) => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        reject(error);
      },
    );
  });
}

export interface MountedAgentSessionSynchronization {
  sessionIds?: readonly string[];
  /** A global reconciliation boundary supersedes any live stream from the
   * previous Runtime/event generation before reading durable truth. */
  ownership?: SessionProjectionSynchronizationOwnership;
}

/** Reconcile one Session through its mounted lifecycle owner and let a command
 * wait for the same authoritative material boundary the event loop uses. */
export async function synchronizeMountedAgentSession(
  sessionId: string,
  ownership?: SessionProjectionSynchronizationOwnership,
): Promise<boolean> {
  const entry = agentSessionView().getSession(sessionId);
  if (!entry) return false;
  if (entry.synchronize) return entry.synchronize(ownership);
  if (ownership === "retire-live") return false;
  return (await refreshAgentSessionProjection(sessionId)) !== null;
}

export function synchronizeMountedAgentSessions(
  request: MountedAgentSessionSynchronization = {},
): readonly string[] {
  const sessions = agentSessionView().getSessions();
  const mountedIds = Object.keys(sessions);
  const requested = request.sessionIds ? new Set(request.sessionIds) : null;
  const targets = requested
    ? mountedIds.filter((sessionId) => requested.has(sessionId))
    : mountedIds;
  if (request.ownership === "replace-live" || request.ownership === "retire-live") {
    agentSessionView().retireProjectionGeneration(targets);
  }
  if (request.ownership === "replace-server") {
    agentSessionView().replaceServerScope(targets);
  }
  for (const sessionId of targets) {
    void synchronizeMountedAgentSession(sessionId, request.ownership);
  }
  return targets;
}
