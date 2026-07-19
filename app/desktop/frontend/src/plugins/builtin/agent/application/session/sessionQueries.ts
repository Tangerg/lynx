import { createDataQuery } from "@/lib/data/dataQuery";
import { queryClient } from "@/lib/data/queryClient";

export interface AgentSessionSummary {
  id: string;
  revision: number;
  title: string;
  status: "running" | "waiting" | "idle";
  model: string;
  cwd?: string;
  cwdMissing?: boolean;
  favorite?: boolean;
  time: string;
}

export const AGENT_SESSIONS_KEY = "sessions";

export const useAgentSessions = createDataQuery<AgentSessionSummary[]>(AGENT_SESSIONS_KEY);

/** Refresh the session collection after a session command succeeds.
 * Cwd-changing commands also refresh the integration-level project index. */
export function invalidateAgentSessions(options?: { projects?: boolean }): Promise<void> {
  const sessions = queryClient.invalidateQueries({ queryKey: [AGENT_SESSIONS_KEY] });
  if (!options?.projects) return sessions;
  return Promise.all([sessions, queryClient.invalidateQueries({ queryKey: ["projects"] })]).then(
    () => undefined,
  );
}
