// Subscribe to agent shared state (state.snapshot) on
// the current session. `useSharedState()` returns the whole document;
// `useSharedState("a.b.c")` traverses dot-segments.

import { agentViewState } from "../application/ports/viewState";

export function useSharedState<T = unknown>(path?: string): T | undefined {
  return agentViewState().useSharedState<T>(path);
}
