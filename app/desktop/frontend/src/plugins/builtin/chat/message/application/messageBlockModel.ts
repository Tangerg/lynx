import type { ContentBlock, MessageRole, ToolCall } from "@/plugins/builtin/agent/public/viewState";
import {
  planRenderUnits,
  type MessageRenderUnit,
} from "@/plugins/builtin/agent/public/messagePresentation";
import type { Citation, CitationSource } from "@/plugins/sdk";

export function messageCitations(
  blocks: ContentBlock[],
  sources: readonly CitationSource[],
): Citation[] {
  return sources
    .flatMap((source) => source(blocks))
    .map((citation, index) => ({
      ...citation,
      index: index + 1,
    }));
}

/**
 * The planner's units, with one presentation rule the planner has no business knowing:
 * a text block that is no longer the last one has stopped streaming whether or not the
 * fold has caught up, and a caret blinking in the middle of a finished turn is a lie.
 */
export function messageBlockRenderUnits(
  blocks: ContentBlock[],
  toolCalls: Record<string, ToolCall>,
): MessageRenderUnit[] {
  const lastIndex = blocks.length - 1;
  return planRenderUnits(blocks, toolCalls).map((unit) => {
    if (unit.kind !== "block") return unit;
    const { block, index } = unit;
    if (block.kind === "text" && block.status === "running" && index !== lastIndex) {
      return { ...unit, block: { ...block, status: "complete" } };
    }
    return unit;
  });
}

export function messageBlocksRenderInstant(role: MessageRole): boolean {
  return role === "user";
}
