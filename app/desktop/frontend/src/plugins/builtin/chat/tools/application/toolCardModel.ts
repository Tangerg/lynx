import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import type { ActivityShell } from "@/lib/activityShell";
import {
  toolActivityShell,
  toolDiffStat,
  toolIntent,
  toolMetaItems,
  type ToolDetail,
  type ToolIntent,
  type ToolMetaItem,
} from "@/plugins/builtin/agent/public/messagePresentation";
import type { ToolActionSpec, ToolViewOpenerSpec } from "@/plugins/sdk";

export interface ToolCardModel {
  running: boolean;
  isError: boolean;
  denied: boolean;
  intent: ToolIntent;
  detail?: ToolDetail;
  /** `+n −m` for a call that changed lines, rendered by the same atom the diff
   *  views use. Absent when there is nothing to report. */
  diffStat?: { added: number; removed: number };
  metaItems: ToolMetaItem[];
  shell: ActivityShell;
  tone: "neutral" | "warning" | "negative";
}

export function toolCardModel(t: Translate, tool: ToolCall): ToolCardModel {
  const isError = tool.status === "err";
  const intent = toolIntent(t, tool);
  const metaItems = toolMetaItems(t, tool);
  const diffStat = toolDiffStat(tool);
  return {
    running: tool.status === "running",
    isError,
    denied: tool.status === "denied",
    intent,
    // An error message replaces the subject: what went wrong outranks what it was
    // going to act on, and a failure is prose, never a path.
    detail: isError && tool.error ? { kind: "text", value: tool.error } : intent.detail,
    diffStat,
    metaItems,
    shell: toolActivityShell(tool),
    // Lifecycle truth is carried by inline text/dot metadata. Colouring the
    // identity glyph turned errors and refusals back into status cards even after
    // the outer fill was removed.
    tone: "neutral",
  };
}

export function toolCardActions(
  tool: ToolCall,
  actions: readonly ToolActionSpec[],
): ToolActionSpec[] {
  return actions.filter((action) => !action.predicate || action.predicate(tool));
}

export function toolCardViewOpener(
  tool: ToolCall,
  openers: readonly ToolViewOpenerSpec[],
): ToolViewOpenerSpec | undefined {
  return openers.find((opener) => opener.predicate(tool));
}

export function visibleToolMetaItems(
  items: readonly ToolMetaItem[],
  running: boolean,
): ToolMetaItem[] {
  return running ? items.filter((item) => item.id !== "live") : [...items];
}
