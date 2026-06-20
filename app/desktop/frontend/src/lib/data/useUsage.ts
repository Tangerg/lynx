// Usage reporting read hooks (usage.session / usage.summary). Parameterized
// reads, so they use TanStack useQuery directly against the runtime client
// rather than the registry-backed makeDataQuery pattern (which keys off a
// constant). Lives in lib/ so panes/components reach the runtime through a hook,
// never @/rpc or the container directly (layer rule).

import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import type { Usage, UsageSummary } from "@/rpc";
import { asSessionId } from "@/rpc";
import { getContainer } from "@/main/container";
import { useAgentRunning } from "@/state/agentStore";
import { queryClient } from "./queryClient";

export const USAGE_SESSION_KEY = "usage.session";
export const USAGE_SUMMARY_KEY = "usage.summary";

/**
 * The active session's cumulative token usage + cost, summed server-side over
 * its finished runs. Refetches when a run finishes (running → idle): a run's
 * terminal metering is what changes the cumulative total.
 */
export function useSessionUsage(sessionId: string | undefined) {
  const running = useAgentRunning();
  const query = useQuery<Usage>({
    queryKey: [USAGE_SESSION_KEY, sessionId],
    queryFn: () =>
      getContainer()
        .client()
        .usage.session(asSessionId(sessionId ?? "")),
    enabled: Boolean(sessionId),
  });

  const wasRunning = useRef(running);
  useEffect(() => {
    if (wasRunning.current && !running && sessionId) {
      void queryClient.invalidateQueries({ queryKey: [USAGE_SESSION_KEY, sessionId] });
    }
    wasRunning.current = running;
  }, [running, sessionId]);

  return query;
}

/**
 * Cross-session spend report (usage.summary). sinceDays limits the window
 * (0 / omitted = all time).
 */
export function useUsageSummary(sinceDays: number) {
  return useQuery<UsageSummary>({
    queryKey: [USAGE_SUMMARY_KEY, sinceDays],
    queryFn: () =>
      getContainer()
        .client()
        .usage.summary(sinceDays > 0 ? { sinceDays } : {}),
  });
}
