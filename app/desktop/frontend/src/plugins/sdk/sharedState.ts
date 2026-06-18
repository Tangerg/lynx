// Subscribe to agent shared state (state.snapshot / state.delta) on
// the current session. `useSharedState()` returns the whole document;
// `useSharedState("a.b.c")` traverses dot-segments.

import { useAgentSlice } from "@/state/agentStore";

export function useSharedState<T = unknown>(path?: string): T | undefined {
  return useAgentSlice<T | undefined>((v) => {
    if (!path) return v.shared as unknown as T;
    let cur: unknown = v.shared;
    for (const seg of path.split(".")) {
      if (cur == null || typeof cur !== "object") return undefined;
      cur = (cur as Record<string, unknown>)[seg];
    }
    return cur as T;
  });
}
