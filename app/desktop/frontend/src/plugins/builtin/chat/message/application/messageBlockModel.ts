import type {
  ContentBlock,
  Message,
  MessageRole,
  ToolCall,
} from "@/plugins/builtin/agent/public/viewState";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { MessageActionMaterialization } from "@/plugins/builtin/chat/message-actions/public/messageActions";
import {
  planRenderUnits,
  type MessageRenderUnit,
} from "@/plugins/builtin/agent/public/messagePresentation";

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
    // Pending questions temporarily own the composer rung, like Codex's native
    // request panel. The same durable block returns here once it settles; only
    // its active presentation moves, so there is still one source and one echo.
    if (block.kind === "question" && block.status === "requires-action" && !block.answered) {
      return false;
    }
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
  answerFollows = false,
): MessageRenderUnit[] {
  const lastIndex = blocks.length - 1;
  return planRenderUnits(blocks, toolCalls, answerFollows).map((unit) => {
    if (unit.kind !== "block") return unit;
    const { block, index } = unit;
    if (block.kind === "text" && block.status === "running" && index !== lastIndex) {
      return { ...unit, block: { ...block, status: "complete" } };
    }
    return unit;
  });
}

/** Whether the next row is the Runtime-authored final answer for this exact work turn. */
export function finalAnswerFollows(message: Message, next?: Message): boolean {
  return (
    message.role === "assistant" &&
    message.phase === "commentary" &&
    message.runId !== null &&
    next?.role === "assistant" &&
    next.phase === "finalAnswer" &&
    next.runId === message.runId
  );
}

export function messageBlocksRenderInstant(role: MessageRole): boolean {
  return role === "user";
}

/**
 * Whether the turn still owns material that can change beneath its action row.
 *
 * Current/root attention is deliberately not part of this value. The exact Run named
 * by the message owns the whole turn: completing one agentMessage Item does not settle
 * controls while that Run can still append another Item, and a successor Run cannot
 * settle its predecessor. Blocks, ToolCalls, delegated Runs, and the visible-text
 * projection then narrow that lifecycle further.
 */
export function messageActionMaterialization(row: TranscriptRow): MessageActionMaterialization {
  if (
    row.message.role === "assistant" &&
    (row.runOwner.kind !== "owned" || row.runOwner.status !== "finished")
  ) {
    return "active";
  }

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
