import { useQuery } from "@tanstack/react-query";
import { agentRuntime } from "../ports/runtimeGateway";

export const AGENT_SESSION_USAGE_KEY = "usage.session";

export function useAgentSessionUsage(sessionId: string | undefined) {
  return useQuery({
    queryKey: [AGENT_SESSION_USAGE_KEY, sessionId],
    queryFn: () => agentRuntime().loadSessionUsage(sessionId ?? ""),
    enabled: Boolean(sessionId),
  });
}
