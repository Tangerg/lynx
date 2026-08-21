import type { AgentPlan, AgentSessionView } from "@/plugins/sdk/types/agentSessionView";

// The revision decides which of two Plan replacements is later. Contents cannot: the list is
// replaced wholesale, so it can shrink, and an out-of-order older snapshot would look
// exactly like progress being undone. Dropping the older one is what makes the fold
// order-insensitive — which it has to be, because a live stream and a cold read can
// both deliver one.
export function onPlanUpdated(state: AgentSessionView, plan: AgentPlan): AgentSessionView {
  if (state.plan && state.plan.revision >= plan.revision) return state;
  return { ...state, plan };
}
