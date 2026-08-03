import type { Translate } from "@/lib/i18n";
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
): MessageRenderUnit[] {
  const hasQuestion = blocks.some((block) => block.kind === "question");
  const answered = answeredAfter(blocks);
  const units: MessageRenderUnit[] = [];
  let wave: PositionedBlock[] = [];

  const flushWave = () => {
    if (wave.length === 0) return;
    const inner = planWithinWave(wave, toolCalls, hasQuestion, answered);
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
 * Whether prose follows each block.
 *
 * Backwards, because the question is about what comes AFTER it. A text block that is
 * still streaming counts: the answer has begun, which is exactly the moment the work
 * behind it should get out of the way.
 */
function answeredAfter(blocks: ContentBlock[]): boolean[] {
  const answered: boolean[] = Array.from({ length: blocks.length }, () => false);
  let seen = false;
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    answered[index] = seen;
    if (blocks[index]!.kind === "text") seen = true;
  }
  return answered;
}

/** Adjacent read-only calls still group inside a wave: they are one act of looking,
 *  whether or not the wave around them is folded. */
function planWithinWave(
  positioned: readonly PositionedBlock[],
  toolCalls: Record<string, ToolCall>,
  hasQuestion: boolean,
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
    if (tool && isReadOnlyTool(tool.name)) {
      reads.push(item);
      continue;
    }
    // A question's own tool call is rendered by the question card, not twice.
    if (tool && hasQuestion && isQuestionTool(tool.name)) {
      flushReads();
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

function toolOf(block: ContentBlock, toolCalls: Record<string, ToolCall>): ToolCall | undefined {
  return block.kind === "tool" ? toolCalls[block.toolCallId] : undefined;
}

/**
 * What a folded wave says about itself.
 *
 * Counts rather than a list of verbs: the row is closed, and the only question a
 * reader asks of it is how much is in there. The words come from the same catalog the
 * tool group's summary uses, so a folded wave and a folded group speak alike.
 */
export function waveSummary(t: Translate, units: readonly MessageRenderUnit[]): string {
  let thought = false;
  let calls = 0;
  for (const unit of units) {
    if (unit.kind === "toolGroup") calls += unit.tools.length;
    else if (unit.kind === "block" && unit.block.kind === "tool") calls += 1;
    else if (unit.kind === "block" && unit.block.kind === "reasoning") thought = true;
  }
  const parts: string[] = [];
  // Whether it thought, not how many times: the number of reasoning blocks a provider
  // happens to emit is an artefact of its streaming, not something a reader can use.
  if (thought) parts.push(t("narrative.wave.thinking"));
  if (calls > 0) parts.push(t("tools.group.calls", { count: calls }));
  return parts.join(" · ");
}
