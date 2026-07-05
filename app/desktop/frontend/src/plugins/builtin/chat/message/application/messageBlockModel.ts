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

export function messageBlockRenderUnits(
  blocks: ContentBlock[],
  toolCalls: Record<string, ToolCall>,
): MessageRenderUnit[] {
  const lastIndex = blocks.length - 1;
  return planRenderUnits(blocks, toolCalls).map((unit) => {
    if (unit.kind === "toolGroup") return unit;
    const { block, index } = unit;
    if (block.kind === "text" && block.status === "running" && index !== lastIndex) {
      return { kind: "block", block: { ...block, status: "complete" }, index };
    }
    return unit;
  });
}

export function messageBlocksRenderInstant(role: MessageRole): boolean {
  return role === "user";
}
