import type { IconName } from "@/ui/icons";
import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { toolIconFor, toolRoutingKey } from "@/plugins/builtin/chat/tools/public/toolIcon";

// Lives in the ui ring, not application: an icon name is a UI vocabulary and
// `toolIconFor` reads the icon registry, so this is presentation glue — the same
// reason `tools/public/toolIcon` sits where it does rather than in a model.

/**
 * The mark of a folded wave: what it opened with.
 *
 * One mark, because the row has one slot for it (see AgentActivityDisclosure's
 * gutter). What the wave DID rides in the label as words, which is a slot
 * that can hold a tally without pushing anything sideways.
 */
export function waveGlyph(
  units: readonly MessageRenderUnit[],
  toolCalls: Record<string, ToolCall>,
): IconName | undefined {
  for (const unit of units) {
    if (unit.kind === "toolGroup") {
      const first = unit.tools[0];
      if (first) return toolIconFor(toolRoutingKey(first));
      continue;
    }
    if (unit.kind !== "block") continue;
    if (unit.block.kind === "reasoning") return "sparkle";
    if (unit.block.kind === "tool") {
      const tool = toolCalls[unit.block.toolCallId];
      if (tool) return toolIconFor(toolRoutingKey(tool));
    }
  }

  return undefined;
}
