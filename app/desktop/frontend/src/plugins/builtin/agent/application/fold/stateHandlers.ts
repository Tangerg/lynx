import type { AgentViewState } from "@/plugins/sdk/types/agentView";

export function onStateSnapshot(
  state: AgentViewState,
  shared: Record<string, unknown>,
): AgentViewState {
  return { ...state, shared };
}
