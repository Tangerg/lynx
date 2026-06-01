// Shared-state handlers — STATE_SNAPSHOT replaces `state.shared`
// wholesale; STATE_DELTA applies a JSON Patch (RFC 6902). A throwing
// patch is logged and the state left unchanged — silently swallowing
// a bad patch beats crashing the chat.

import type { StateDeltaEvent, StateSnapshotEvent } from "@ag-ui/core";
import type { Operation } from "fast-json-patch";
import type { AgentViewState } from "@/protocol/agui/viewState";
import { applyPatch, deepClone } from "fast-json-patch";

export const onStateSnapshot = (state: AgentViewState, ev: StateSnapshotEvent): AgentViewState => {
  const snapshot = (ev as { snapshot?: unknown }).snapshot;
  if (snapshot == null || typeof snapshot !== "object") return state;
  return { ...state, shared: snapshot as Record<string, unknown> };
};

export const onStateDelta = (state: AgentViewState, ev: StateDeltaEvent): AgentViewState => {
  const patch = (ev as { delta?: unknown[] }).delta;
  if (!Array.isArray(patch) || patch.length === 0) return state;
  try {
    // Clone to keep our `shared` immutable across the reduction step
    // — fast-json-patch mutates the document in place otherwise.
    const next = applyPatch(
      deepClone(state.shared),
      patch as Operation[],
      /* validate */ false,
      /* mutateDocument */ true,
    ).newDocument as Record<string, unknown>;
    return { ...state, shared: next };
  } catch (err) {
    console.error("[agui] STATE_DELTA patch failed:", err);
    return state;
  }
};
