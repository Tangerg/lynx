import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import type { ActivityShell } from "@/lib/activityShell";
import {
  toolActivityShell,
  toolIntent,
  toolMetaItems,
  type ToolIntent,
  type ToolMetaItem,
} from "@/plugins/builtin/agent/public/messagePresentation";
import type { ToolActionSpec, ToolViewOpenerSpec } from "@/plugins/sdk";

export interface ToolCardModel {
  running: boolean;
  isError: boolean;
  denied: boolean;
  intent: ToolIntent;
  detail?: string;
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
  return {
    running: tool.status === "running",
    isError,
    denied: tool.status === "denied",
    intent,
    detail: isError && tool.error ? tool.error : intent.detail,
    metaItems,
    shell: toolActivityShell(tool),
    tone: toolCardTone(tool),
    showSettledMark: tool.status === "ok" && metaItems.length === 0,
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
