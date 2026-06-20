// Usage reporting read hooks (usage.session / usage.summary). Parameterized
// reads, so they use TanStack useQuery directly against the runtime client
// rather than the registry-backed makeDataQuery pattern (which keys off a
// constant). Lives in lib/ so panes/components reach the runtime through a hook,
// never @/rpc or the container directly (layer rule).

import { useQuery } from "@tanstack/react-query";
import type { Usage, UsageSummary } from "@/rpc";
import { asSessionId } from "@/rpc";
import { getContainer } from "@/main/container";

export const USAGE_SESSION_KEY = "usage.session";
export const USAGE_SUMMARY_KEY = "usage.summary";

/**
 * A session's cumulative token usage + cost, summed server-side over its
 * finished runs. Freshness is driven by the run pump: the run driver
 * (useAgentSession) invalidates [USAGE_SESSION_KEY, sessionId] when a
 * run.finished folds for that session — including a session running in the
 * background — so the chip updates on the authoritative "metering durable"
 * signal rather than guessing from the active session's running flag.
 */
export function useSessionUsage(sessionId: string | undefined) {
  return useQuery<Usage>({
    queryKey: [USAGE_SESSION_KEY, sessionId],
    queryFn: () =>
      getContainer()
        .client()
        .usage.session(asSessionId(sessionId ?? "")),
    enabled: Boolean(sessionId),
  });
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
