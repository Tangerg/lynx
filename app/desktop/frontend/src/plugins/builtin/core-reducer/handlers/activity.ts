// Per-message activity-stream handlers — ACTIVITY_SNAPSHOT writes the
// content blob (with an optional `replace` flag controlling
// replace-vs-merge); ACTIVITY_DELTA applies a JSON Patch. Renderers
// pick the activity types they understand and ignore the rest.

import type { ActivityDeltaEvent, ActivitySnapshotEvent } from "@ag-ui/core";
import type { Operation } from "fast-json-patch";
import type { AgentViewState } from "@/protocol/agui/viewState";
import { applyPatch, deepClone } from "fast-json-patch";
import { updateActivity } from "../helpers";

export const onActivitySnapshot = (
  state: AgentViewState,
  ev: ActivitySnapshotEvent,
): AgentViewState => {
  const content = (ev as { content?: Record<string, unknown> }).content ?? {};
  const replace = Boolean((ev as { replace?: boolean }).replace);
  return updateActivity(state, ev.messageId, ev.activityType, (prev) => {
    if (replace || !prev || typeof prev !== "object") return content;
    return { ...(prev as Record<string, unknown>), ...content };
  });
};

export const onActivityDelta = (state: AgentViewState, ev: ActivityDeltaEvent): AgentViewState => {
  const patch = (ev as { patch?: unknown[] }).patch;
  if (!Array.isArray(patch) || patch.length === 0) return state;
  return updateActivity(state, ev.messageId, ev.activityType, (prev) => {
    try {
      const base = prev && typeof prev === "object" ? deepClone(prev) : {};
      return applyPatch(base, patch as Operation[], false, true).newDocument;
    } catch (err) {
      console.error(
        `[agui] ACTIVITY_DELTA patch failed for ${ev.messageId}/${ev.activityType}:`,
        err,
      );
      return prev;
    }
  });
};
