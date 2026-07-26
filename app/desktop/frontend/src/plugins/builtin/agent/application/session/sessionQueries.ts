import { createDataQuery } from "@/plugins/sdk";
import { queryClient } from "@/lib/queryClient";

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

/** Refresh the session collection after a session command succeeds. Views derived
 *  from it — the project index — watch this key and refresh themselves. */
export function invalidateAgentSessions(): Promise<void> {
  return queryClient.invalidateQueries({ queryKey: [AGENT_SESSIONS_KEY] });
}
