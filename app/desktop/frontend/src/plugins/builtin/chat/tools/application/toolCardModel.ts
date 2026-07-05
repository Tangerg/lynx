import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import {
  toolIntent,
  toolMetaItems,
  type ToolIntent,
  type ToolMetaItem,
} from "@/plugins/builtin/agent/public/messagePresentation";
import type { ToolActionSpec, ToolViewOpenerSpec } from "@/plugins/sdk";

export interface ToolCardModel {
  running: boolean;
  isError: boolean;
  needsAction: boolean;
  intent: ToolIntent;
  detail?: string;
  metaItems: ToolMetaItem[];
}

export function toolCardModel(tool: ToolCall): ToolCardModel {
  const isError = tool.status === "err";
  const intent = toolIntent(tool);
  return {
    running: tool.status === "running",
    isError,
    needsAction: tool.status === "requires-action",
    intent,
    detail: isError && tool.error ? tool.error : intent.detail,
    metaItems: toolMetaItems(tool),
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
