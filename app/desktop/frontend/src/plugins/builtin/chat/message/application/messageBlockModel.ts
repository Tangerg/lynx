import type { ContentBlock, MessageRole, ToolCall } from "@/plugins/builtin/agent/public/viewState";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { MessageActionMaterialization } from "@/plugins/builtin/chat/message-actions/public/messageActions";
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
 * The blocks the narrative tells, which is not every block the turn produced: a tool
 * whose outcome is held by a surface that stays on screen has nothing left to say here.
 *
 * Dropped before planning rather than skipped while rendering, because the units carry
 * counts and grouping — a folded wave that says "4 steps" and shows 3, or a run of
 * adjacent reads broken by a row nobody can see, are both worse than the duplication.
 * The call itself is untouched: it is still in `toolCalls`, so Tool stats and the
 * timeline still account for it.
 */
export function narratedBlocks(
  blocks: ContentBlock[],
  toolCalls: Record<string, ToolCall>,
  standing: (toolName: string) => boolean,
): ContentBlock[] {
  return blocks.filter((block) => {
    if (block.kind !== "tool") return true;
    const name = toolCalls[block.toolCallId]?.name;
    return name === undefined || !standing(name);
  });
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

/**
 * Whether the turn still owns material that can change beneath its action row.
 *
 * Root attention is deliberately not part of this value. A connection transition can
 * briefly lose the active Run before snapshot/event recovery reattaches it, while the
 * turn's own block, ToolCall, or delegated Run remains explicitly non-terminal. Those
 * item-local facts are the durable authority for whether terminal actions may exist.
 */
export function messageActionMaterialization(row: TranscriptRow): MessageActionMaterialization {
  for (const block of row.message.blocks) {
    if (blockOwnsActiveMaterial(block)) return "active";
    if (block.kind !== "tool") continue;
    const call = row.facts.toolCalls[block.toolCallId];
    if (call?.status === "running" || call?.status === "requires-action") return "active";
  }

  for (const narratives of Object.values(row.facts.delegatedRuns)) {
    if (narratives.some(({ run }) => run.status !== "finished")) return "active";
  }

  return "settled";
}

function blockOwnsActiveMaterial(block: ContentBlock): boolean {
  switch (block.kind) {
    case "text":
    case "reasoning":
    case "approval":
    case "question":
      return block.status === "running" || block.status === "requires-action";
    default:
      return false;
  }
}
