import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { isQuestionTool } from "../domain/toolCategory";
import { isReadOnlyTool } from "./toolPresentation";

/**
 * Whether prose already follows this unit — the work an answer speaks for.
 *
 * Carried on the unit rather than recomputed by the renderer, because the planner is
 * already asking the same question to decide what to fold: a unit that is superseded
 * and stands alone (a lone reasoning block, a read group that folds itself) needs to
 * render closed too, and two places deriving one rule is how they drift apart.
 *
 * A `wave` needs no flag: it exists only because an answer followed it.
 */
type Superseded = { superseded: boolean };

export type MessageRenderUnit =
  | ({ kind: "block"; block: ContentBlock; index: number } & Superseded)
  | ({ kind: "toolGroup"; tools: ToolCall[] } & Superseded)
  | { kind: "wave"; units: MessageRenderUnit[] };

/**
 * What a turn DOES between answers: thinking, and calling tools.
 *
 * Approvals and questions are not process even though they arrive mid-turn — they are
 * asking the reader for something, and folding away a request for a decision is how a
 * turn ends in silence. Nor is a plan or an image: those are read, not performed.
 */
function isProcess(block: ContentBlock): boolean {
  return block.kind === "reasoning" || block.kind === "tool";
}

interface PositionedBlock {
  block: ContentBlock;
  index: number;
}

/**
 * Plans one message's blocks into the units the transcript renders.
 *
 * A turn alternates between doing and answering, and it is the ANSWERS a reader came
 * for. So every run of process blocks that already has prose after it folds into a
 * single `wave` — the account of how that answer was reached, one row, openable — and
 * a long turn reads as work · answer · work · answer instead of as everything the
 * agent ever did, all at the same weight.
 *
 * The run still in flight is never folded: that is the one the reader is watching.
 */
export function planRenderUnits(
  blocks: ContentBlock[],
  toolCalls: Record<string, ToolCall>,
  answerFollows = false,
): MessageRenderUnit[] {
  const hasQuestion = blocks.some((block) => block.kind === "question");
  const approvalOwnedToolCallIds = findApprovalOwnedToolCallIds(blocks, toolCalls);
  const answered = answeredAfter(blocks, answerFollows);
  const units: MessageRenderUnit[] = [];
  let wave: PositionedBlock[] = [];

  const flushWave = () => {
    if (wave.length === 0) return;
    const inner = planWithinWave(wave, toolCalls, hasQuestion, approvalOwnedToolCallIds, answered);
    const last = wave[wave.length - 1]!;
    // Two units minimum: a run that already plans to one row — a lone reasoning block,
    // or three reads that group themselves — folds on its own, and wrapping it would
    // only add a level to open through.
    if (answered[last.index] && inner.length >= 2) units.push({ kind: "wave", units: inner });
    else units.push(...inner);
    wave = [];
  };

  blocks.forEach((block, index) => {
    if (isProcess(block)) {
      wave.push({ block, index });
      return;
    }
    flushWave();
    units.push({ kind: "block", block, index, superseded: answered[index]! });
  });

  flushWave();
  return units;
}

/**
 * Whether an answer follows each block.
 *
 * Backwards, because the question is about what comes AFTER it. A text block that is
 * still streaming counts: the answer has begun, which is exactly the moment the work
 * behind it should get out of the way.
 *
 * It has to carry TEXT to count. `item.started` creates the answer's block before a
 * token of it exists, so treating the block's presence as the answer folded the
 * thinking away the instant the model opened its reply — to a one-line row with
 * nothing in it, while the answer it supposedly deferred to was still empty. For a
 * provider that streams reasoning and prose from overlapping items, that is the whole
 * time the model is thinking.
 */
function answeredAfter(blocks: ContentBlock[], answerFollows: boolean): boolean[] {
  const answered: boolean[] = Array.from({ length: blocks.length }, () => false);
  let seen = answerFollows;
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    answered[index] = seen;
    const block = blocks[index]!;
    if (block.kind === "text" && block.text.trim() !== "") seen = true;
  }
  return answered;
}

/** Adjacent read-only calls still group inside a wave: they are one act of looking,
 *  whether or not the wave around them is folded. */
function planWithinWave(
  positioned: readonly PositionedBlock[],
  toolCalls: Record<string, ToolCall>,
  hasQuestion: boolean,
  approvalOwnedToolCallIds: ReadonlySet<string>,
  answered: readonly boolean[],
): MessageRenderUnit[] {
  const units: MessageRenderUnit[] = [];
  let reads: PositionedBlock[] = [];

  const flushReads = () => {
    if (reads.length >= 2) {
      units.push({
        kind: "toolGroup",
        tools: reads.map((item) => toolOf(item.block, toolCalls)!),
        superseded: answered[reads[reads.length - 1]!.index]!,
      });
    } else {
      for (const item of reads) {
        units.push({
          kind: "block",
          block: item.block,
          index: item.index,
          superseded: answered[item.index]!,
        });
      }
    }
    reads = [];
  };

  for (const item of positioned) {
    const tool = toolOf(item.block, toolCalls);
    // The fold deliberately retains both facts while a call waits: the ToolCall is
    // the durable operation, and the approval block is the actionable interruption.
    // The transcript must still expose one request surface. Match by the runtime's
    // shared item id and only while both facts are pending, so the historical tool row
    // returns as soon as the decision settles.
    if (tool && approvalOwnedToolCallIds.has(tool.id)) {
      flushReads();
      continue;
    }
    // A question's own tool call is rendered by the question card, not twice. Checked
    // ahead of the grouping: these tools are side-effect-free (they ARE the
    // interrupt), so they read as glances and would otherwise be folded into a group
    // instead of dropped.
    if (tool && hasQuestion && isQuestionTool(tool.name)) {
      flushReads();
      continue;
    }
    if (tool && isReadOnlyTool(tool)) {
      reads.push(item);
      continue;
    }
    flushReads();
    units.push({
      kind: "block",
      block: item.block,
      index: item.index,
      superseded: answered[item.index]!,
    });
  }

  flushReads();
  return units;
}

function findApprovalOwnedToolCallIds(
  blocks: readonly ContentBlock[],
  toolCalls: Record<string, ToolCall>,
): ReadonlySet<string> {
  const ids = new Set<string>();
  for (const block of blocks) {
    if (block.kind !== "approval" || block.status !== "requires-action" || !block.itemId) {
      continue;
    }
    if (toolCalls[block.itemId]?.status === "requires-action") ids.add(block.itemId);
  }
  return ids;
}

function toolOf(block: ContentBlock, toolCalls: Record<string, ToolCall>): ToolCall | undefined {
  return block.kind === "tool" ? toolCalls[block.toolCallId] : undefined;
}

/**
 * How much is inside a folded wave — every step of it, thinking included.
 *
 * One total rather than a breakdown, and a count rather than a verb. The first version
 * of this row said "Working · thinking · 4 calls", which was wrong twice: a folded wave
 * exists only because an answer followed it, so it is always PAST and never working;
 * and the number counted tool calls alone, so a round of four commands and two
 * conclusions claimed to hold four things.
 */
export function waveStepCount(units: readonly MessageRenderUnit[]): number {
  let steps = 0;
  for (const unit of units) {
    if (unit.kind === "toolGroup") steps += unit.tools.length;
    else if (unit.kind === "block" && unit.block.kind === "tool") steps += 1;
    else if (unit.kind === "block" && unit.block.kind === "reasoning") steps += 1;
  }
  return steps;
}

/**
 * The calls a folded wave holds, in the order it made them.
 *
 * So the row can say what it DID rather than only how many steps it took. Kept
 * beside the count for the same reason the count is here: both are questions about
 * a planned unit, and the planner is the only thing that knows a wave's members
 * without walking blocks a second time. The thinking a wave also holds cannot be
 * classified into an act, which is exactly why the count stays.
 */
export function waveToolCalls(
  units: readonly MessageRenderUnit[],
  toolCalls: Record<string, ToolCall>,
): ToolCall[] {
  const tools: ToolCall[] = [];
  for (const unit of units) {
    if (unit.kind === "toolGroup") {
      tools.push(...unit.tools);
      continue;
    }
    if (unit.kind !== "block" || unit.block.kind !== "tool") continue;
    const tool = toolCalls[unit.block.toolCallId];
    if (tool) tools.push(tool);
  }
  return tools;
}
