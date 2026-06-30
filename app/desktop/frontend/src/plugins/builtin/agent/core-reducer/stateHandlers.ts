import type { Operation } from "fast-json-patch";
import type { AgentViewState } from "@/protocol/run/viewState";
import { applyPatch, deepClone } from "fast-json-patch";

export function onStateSnapshot(
  state: AgentViewState,
  shared: Record<string, unknown>,
): AgentViewState {
  return { ...state, shared };
}

export function onStateDelta(state: AgentViewState, patch: Operation[]): AgentViewState {
  try {
    const next = applyPatch(deepClone(state.shared), patch, false, false).newDocument;
    return { ...state, shared: next as Record<string, unknown> };
  } catch (err) {
    console.error("[core-reducer] state.delta patch failed:", err);
    return state;
  }
}
