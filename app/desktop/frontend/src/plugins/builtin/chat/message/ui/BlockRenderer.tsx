// Message block dispatcher — maps each ContentBlock (text, tool, reasoning,
// approval, question, compaction, search, code, checkpoint) to its React card.
import type { ContentBlock } from "@/plugins/builtin/agent/public/viewState";
import type { TranscriptRow, TurnFacts } from "@/plugins/builtin/agent/public/conversation";
import type { MessageRenderUnit } from "@/plugins/builtin/agent/public/messagePresentation";
import type { BlockCtx } from "./blockContext";
export type { BlockCtx } from "./blockContext";
import { cn } from "@/lib/classNames";
import { MarkdownMessage } from "./markdown/MarkdownMessage";
import { ApprovalCard, CompactionBlock, ImageBlock, QuestionCard, ReasoningBlock } from "./cards";
import { ToolCard, ToolGroup } from "@/plugins/builtin/chat/tools/public/rendering";
import { lookupExtensionByKey } from "@/plugins/sdk";
import { TOOL_STANDING_SURFACE } from "@/plugins/sdk/kernelPoints";
import { messageBlockRenderUnits, narratedBlocks } from "../application/messageBlockModel";
import { BLOCK_ANCHOR_ATTR, renderUnitAnchor } from "../application/renderUnitAnchor";
import { unitIndentClass, unitSeamClass } from "../application/renderUnitRhythm";
import { DelegatedNarrative } from "./DelegatedNarrative";
import { NarrativeWave } from "./NarrativeWave";

/**
 * Render one content block.
 *
 * Every protocol content block is rendered directly by this module from its
 * own `cards/` + `markdown/` sub-modules. The message module owns the exact
 * closed union that the Runtime fold produces.
 */
export function renderBlock(
  block: ContentBlock,
  key: number,
  facts: TurnFacts,
  ctx: BlockCtx,
  // Whether the turn has already started answering past this block. Only the blocks
  // that decide their OWN open state read it — see messageBlockRenderUnits.
  superseded = false,
) {
  switch (block.kind) {
    case "text":
      // Wrapper is a <div>, not a <p>: react-markdown emits <p> nodes
      // of its own, and `<p>` inside `<p>` is invalid HTML (browsers
      // silently split the outer one).
      return (
        <div key={key}>
          {ctx.textReveal === "instant" ? (
            <MarkdownMessage text={block.text} reveal="instant" />
          ) : (
            <MarkdownMessage
              text={block.text}
              streaming={block.status === "running"}
              reveal={ctx.textReveal}
            />
          )}
        </div>
      );

    case "image":
      return <ImageBlock key={key} mime={block.mime} data={block.data} />;

    case "tool": {
      const tool = facts.toolCalls[block.toolCallId];
      if (!tool) return null;
      const delegatedRuns = facts.delegatedRuns[block.toolCallId] ?? [];
      return (
        <div id={block.toolCallId} key={block.toolCallId}>
          <ToolCard
            tool={tool}
            expanded={ctx.expandedIds.has(block.toolCallId)}
            onToggleExpand={() => {
              ctx.onSelectTool(block.toolCallId);
              ctx.onToggleExpand(block.toolCallId);
            }}
          />
          {delegatedRuns.map((narrative, index) => (
            <DelegatedNarrative
              key={narrative.run.id}
              narrative={narrative}
              ordinal={index + 1}
              siblingCount={delegatedRuns.length}
              facts={facts}
              ctx={ctx}
              renderMessageBlocks={renderMessageBlocks}
            />
          ))}
        </div>
      );
    }

    case "reasoning":
      return (
        <ReasoningBlock key={key} text={block.text} status={block.status} superseded={superseded} />
      );

    case "approval":
      // Identity key, NOT the block index: HITL cards hold per-interrupt
      // local state (remember / edited args / answers). Index keying reuses
      // the component instance when a different approval lands at the same
      // position, leaking one interrupt's draft state into the next.
      return (
        <ApprovalCard
          key={block.itemId ?? key}
          status={block.status}
          toolName={block.toolName}
          cmd={block.command}
          reason={block.reason}
          runId={block.runId}
          itemId={block.itemId}
          decision={block.decision}
          args={block.args}
          rememberable={block.rememberable}
        />
      );

    case "question":
      // Identity key — same reasoning as the approval card above.
      return (
        <QuestionCard
          key={block.itemId ?? key}
          status={block.status}
          runId={block.runId}
          itemId={block.itemId}
          questions={block.questions}
          answered={block.answered}
          answers={block.answers}
        />
      );

    case "compaction":
      return (
        <CompactionBlock
          key={key}
          summary={block.summary}
          droppedMessages={block.droppedMessages}
        />
      );
  }
}

/**
 * Render a message's blocks, each in an anchored wrapper.
 *
 * The wrapper carries the vertical rhythm, and this is the only place that can: a
 * seam is a relationship between two units, so no card can know its own distance
 * from a neighbour it has never heard of (see renderUnitRhythm). It also exists so the narrative outline can scroll
 * to a block without every card learning about anchors, and it is where the React
 * key lives, which is what makes `renderUnitAnchor`'s identity rule apply to every
 * block kind at once instead of to the two that remembered to ask for it.
 */
/**
 * Render one planned unit. Shared by the transcript and by a folded wave, which holds
 * the same units it would otherwise have rendered inline.
 */
export function renderUnit(unit: MessageRenderUnit, facts: TurnFacts, ctx: BlockCtx) {
  if (unit.kind === "wave")
    return <NarrativeWave units={unit.units} facts={facts} ctx={ctx} renderUnit={renderUnit} />;
  if (unit.kind === "toolGroup") {
    return (
      <ToolGroup
        tools={unit.tools}
        onSelectTool={ctx.onSelectTool}
        expandedIds={ctx.expandedIds}
        onToggleExpand={ctx.onToggleExpand}
        superseded={unit.superseded}
      />
    );
  }
  return renderBlock(unit.block, unit.index, facts, ctx, unit.superseded);
}

// Whether a tool has a surface of its own that stays on screen. Read here rather than
// inside the projection so the projection stays a function of its arguments.
const standingTool = (name: string) =>
  lookupExtensionByKey(TOOL_STANDING_SURFACE, name) !== undefined;

export function renderMessageBlocks(
  { message, facts }: Pick<TranscriptRow, "message" | "facts">,
  ctx: BlockCtx,
  answerFollows = false,
) {
  const units = messageBlockRenderUnits(
    narratedBlocks(message.blocks, facts.toolCalls, standingTool),
    facts.toolCalls,
    answerFollows,
  );
  return units.map((unit, index) => {
    const anchor = renderUnitAnchor(message.id, unit);
    return (
      <div
        key={anchor}
        {...{ [BLOCK_ANCHOR_ATTR]: anchor }}
        className={cn(unitSeamClass(units[index - 1], unit), unitIndentClass(unit))}
      >
        {renderUnit(unit, facts, ctx)}
      </div>
    );
  });
}
