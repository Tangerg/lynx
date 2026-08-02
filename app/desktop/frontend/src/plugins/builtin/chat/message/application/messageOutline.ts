import type { Message, ToolCall } from "@/plugins/builtin/agent/public/viewState";
import {
  summarizeToolGroup,
  toolIntent,
  type MessageRenderUnit,
} from "@/plugins/builtin/agent/public/messagePresentation";
import type { Translate } from "@/lib/i18n";
import { messageBlockRenderUnits } from "./messageBlockModel";
import { renderUnitAnchor } from "./renderUnitAnchor";

export interface MessageOutlineEntry {
  /** Matches the rendered block's `BLOCK_ANCHOR_ATTR`. */
  anchor: string;
  label: string;
}

/** First ATX heading in a markdown body — the author's own name for the section
 *  that follows, which beats anything a generic label could say about it. */
function headingOf(text: string): string | undefined {
  const match = /^\s{0,3}#{1,6}\s+(.+?)\s*#*\s*$/m.exec(text);
  return match?.[1]?.trim() || undefined;
}

/**
 * A long answer's table of contents.
 *
 * Prose is listed only where it names itself with a heading. A turn typically
 * opens and closes with a paragraph or two of connective text, and an outline
 * that entered "Response" three times for those would push the entries that
 * matter — the plan, the command awaiting approval, the diff — off the rail, and
 * would say nothing about where any of them are.
 */
export function messageOutline(
  t: Translate,
  message: Message,
  toolCalls: Record<string, ToolCall>,
): MessageOutlineEntry[] {
  const entries: MessageOutlineEntry[] = [];
  for (const unit of messageBlockRenderUnits(message.blocks, toolCalls)) {
    const label = outlineLabel(t, unit, toolCalls);
    if (label) entries.push({ anchor: renderUnitAnchor(message.id, unit), label });
  }
  return entries;
}

function outlineLabel(
  t: Translate,
  unit: MessageRenderUnit,
  toolCalls: Record<string, ToolCall>,
): string | undefined {
  if (unit.kind === "toolGroup") return summarizeToolGroup(t, unit.tools);

  const { block } = unit;
  switch (block.kind) {
    case "text":
      return headingOf(block.text);
    case "reasoning":
      return t("outline.entry.reasoning");
    case "plan":
      return t("outline.entry.plan");
    case "approval":
      return t("outline.entry.approval");
    case "question":
      return t("outline.entry.question");
    case "compaction":
      return t("outline.entry.compaction");
    case "image":
      return t("outline.entry.image");
    case "tool": {
      const tool = toolCalls[block.toolCallId];
      if (!tool) return undefined;
      const intent = toolIntent(t, tool);
      return intent.detail ? `${intent.label} · ${intent.detail}` : intent.label;
    }
    // Plugin-contributed blocks name themselves through their own registry entry,
    // which this model cannot read without becoming a renderer. They stay off the
    // rail rather than appearing as an unlabelled stop.
    default:
      return undefined;
  }
}
