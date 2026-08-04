import type { IconName } from "@/ui/icons";
import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { toolIconFor, toolRoutingKey } from "@/plugins/builtin/chat/tools/public/toolIcon";

// Lives in the ui ring, not application: an icon name is a UI vocabulary and
// `toolIconFor` reads the icon registry, so this is presentation glue — the same
// reason `tools/public/toolIcon` sits where it does rather than in a model.

/** Past five marks the strip stops being a shape and starts being a queue. */
const MAX_GLYPHS = 5;

/**
 * The marks of what a folded wave holds, in the order it happened.
 *
 * A count alone says how much is inside; these say what KIND — looked, then ran,
 * then changed a file — which is the question a reader actually has before deciding
 * whether to open a row. Free information: every mark is the icon that member would
 * have rendered inline anyway.
 *
 * Runs of the same mark collapse, and that is the whole trick. Four reads in a row
 * as four identical glyphs is a texture, not a fact; one read glyph followed by a
 * command glyph is a story. The count beside the strip is what carries "how many".
 */
export function waveGlyphs(
  units: readonly MessageRenderUnit[],
  toolCalls: Record<string, ToolCall>,
): IconName[] {
  const glyphs: IconName[] = [];
  const push = (icon: IconName) => {
    if (glyphs.length >= MAX_GLYPHS) return;
    if (glyphs[glyphs.length - 1] === icon) return;
    glyphs.push(icon);
  };

  for (const unit of units) {
    if (unit.kind === "toolGroup") {
      for (const tool of unit.tools) push(toolIconFor(toolRoutingKey(tool)));
      continue;
    }
    if (unit.kind !== "block") continue;
    if (unit.block.kind === "reasoning") {
      push("sparkle");
      continue;
    }
    if (unit.block.kind === "tool") {
      const tool = toolCalls[unit.block.toolCallId];
      if (tool) push(toolIconFor(toolRoutingKey(tool)));
    }
  }

  return glyphs;
}
