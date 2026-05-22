// Read AG-UI shared state from the current session's view.
//
// Backends maintain a free-form JSON document via STATE_SNAPSHOT (full
// replace) and STATE_DELTA (JSON Patch). The reducer keeps the current
// document under `viewState.shared`; this hook is the plugin-facing
// way to subscribe.
//
// Two call modes:
//   useSharedState()            → the whole document
//   useSharedState("a.b.c")     → state.shared.a.b.c (or undefined)
//
// Path uses dot-segments. For paths containing literal dots, callers
// should select the whole document and traverse manually — keeping the
// API trivial.

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
