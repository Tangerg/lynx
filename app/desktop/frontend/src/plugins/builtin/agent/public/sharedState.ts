// Subscribe to agent shared state (state.snapshot) on
// the current session. `useSharedState()` returns the whole document;
// `useSharedState("a.b.c")` traverses dot-segments.

import { agentSessionView } from "../application/ports/sessionView";

export function useSharedState<T = unknown>(path?: string): T | undefined {
  return agentSessionView().useSharedState<T>(path);
}
