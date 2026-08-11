import type { AgentRunFailureOutcome, AgentRunOutcome } from "@/plugins/sdk/types/agentSessionView";

export function isAgentRunFailure(
  outcome: AgentRunOutcome | null | undefined,
): outcome is AgentRunFailureOutcome {
  return outcome?.type === "timedOut" || outcome?.type === "failed" || outcome?.type === "lost";
}
