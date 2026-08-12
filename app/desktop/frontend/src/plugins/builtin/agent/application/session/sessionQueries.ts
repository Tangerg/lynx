import { createDataQuery } from "@/plugins/sdk";
import { queryClient } from "@/lib/queryClient";

export interface AgentSessionSummary {
  id: string;
  revision: number;
  title: string;
  status: "running" | "waiting" | "idle";
  model: string;
  cwd: string;
  cwdMissing?: boolean;
  favorite?: boolean;
  time: string;
}

export const AGENT_SESSIONS_KEY = "sessions";

export const useAgentSessions = createDataQuery<AgentSessionSummary[]>(AGENT_SESSIONS_KEY);

/** Refresh the session collection after a session command succeeds. */
export function invalidateAgentSessions(): Promise<void> {
  return queryClient.invalidateQueries({ queryKey: [AGENT_SESSIONS_KEY] });
}

/**
 * Observe one semantic projection of the Session collection.
 *
 * TanStack Query emits cache events for observer attachment, option changes,
 * fetch state, invalidation, and successful data writes. Consumers that react
 * to every one of those internal events create feedback loops: a Session
 * rerender can invalidate an unrelated query, whose rerender updates the
 * Session observer again. Project first and notify only when that value moves.
 */
export function subscribeAgentSessionProjection<T>(
  project: (sessions: readonly AgentSessionSummary[] | undefined) => T,
  onChange: (projection: T) => void,
  equal: (left: T, right: T) => boolean = Object.is,
): () => void {
  let current = project(queryClient.getQueryData<AgentSessionSummary[]>([AGENT_SESSIONS_KEY]));
  return queryClient.getQueryCache().subscribe((event) => {
    if (event.query.queryKey.length !== 1 || event.query.queryKey[0] !== AGENT_SESSIONS_KEY) {
      return;
    }
    const next = project(event.query.state.data as AgentSessionSummary[] | undefined);
    if (equal(current, next)) return;
    current = next;
    onChange(next);
  });
}
