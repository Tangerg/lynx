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
  /** A settled call with nothing to report still has to look settled. Where there
   *  IS something — hits, files, an exit code, a truncation — that is the verdict
   *  and the tick is one more identical glyph in a column of them. */
  showSettledMark: boolean;
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
    tone: toolCardTone(tool),
    // The tick means "nothing to report". A diffstat is something to report, and
    // it left the chip list when it stopped being two chips — so it has to be
    // counted here or an edit would get both.
    showSettledMark: tool.status === "ok" && metaItems.length === 0 && diffStat === undefined,
  };
}

/** Denied used to fall through to neutral, which painted a refused call the same
 *  as a successful one — the `stop` glyph was the only difference, and at 12px it
 *  is not one you notice while scrolling. */
function toolCardTone(tool: ToolCall): "neutral" | "warning" | "negative" {
  if (tool.status === "err") return "negative";
  if (tool.status === "requires-action" || tool.status === "denied") return "warning";
  return "neutral";
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
