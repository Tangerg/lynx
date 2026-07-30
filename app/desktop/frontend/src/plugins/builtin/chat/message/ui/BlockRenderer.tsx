// Message block dispatcher — maps each ContentBlock (text, tool, reasoning,
// approval, question, plan, compaction, search, code, checkpoint) to its React
// card.
import type { ContentBlock, Message } from "@/plugins/builtin/agent/public/viewState";
import type { BlockCtx } from "./blockContext";
export type { BlockCtx } from "./blockContext";
import { MarkdownMessage } from "./markdown/MarkdownMessage";
import {
  ApprovalCard,
  CompactionBlock,
  ImageBlock,
  PlanBlock,
  QuestionCard,
  ReasoningBlock,
} from "./cards";
import { ToolCard, ToolGroup } from "@/plugins/builtin/chat/tools/public/rendering";
import { PluginContentBlock } from "@/plugins/host/PluginContentBlock";
import { messageBlockRenderUnits } from "../application/messageBlockModel";
import { DelegatedNarrative } from "./DelegatedNarrative";

/**
 * Render one content block.
 *
 * Every `BuiltinContentBlockMap` kind — the enumerable, protocol-first-class
 * blocks (text / tool / reasoning / plan / approval / question) — is rendered
 * directly by this module from its own `cards/` + `markdown/` sub-modules. No
 * registry hop: the message module owns the rendering of the blocks the fold
 * produces. `CONTENT_BLOCK` registry / `PluginContentBlock` is reserved for
 * `CustomContentBlockMap` kinds — third-party plugins + the quarantined
 * preview-blocks (search / code / checkpoint) — which fall through to default.
 */
export function renderBlock(block: ContentBlock, key: number, ctx: BlockCtx) {
  switch (block.kind) {
    case "text":
      // Wrapper is a <div>, not a <p>: react-markdown emits <p> nodes
      // of its own, and `<p>` inside `<p>` is invalid HTML (browsers
      // silently split the outer one).
      return (
        <div key={key}>
          <MarkdownMessage
            text={block.text}
            streaming={block.status === "running"}
            instant={ctx.instant}
            typewriter={ctx.typewriter}
          />
        </div>
      );

    case "image":
      return <ImageBlock key={key} mime={block.mime} data={block.data} />;

    case "tool": {
      const tool = ctx.toolCalls[block.toolCallId];
      if (!tool) return null;
      const delegatedRuns = ctx.delegatedRunsByItemId[block.toolCallId] ?? [];
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
              ctx={ctx}
              renderMessageBlocks={renderMessageBlocks}
            />
          ))}
        </div>
      );
    }

    case "reasoning":
      return <ReasoningBlock key={key} text={block.text} status={block.status} />;

    case "plan":
      // The plan block is a "render the current plan here" marker; the data
      // rides view.plan (threaded through ctx), updated by the fold in place.
      return <PlanBlock key={key} plan={ctx.plan} />;

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
          risk={block.risk}
          rememberable={block.rememberable}
          scope={block.scope}
          target={block.target}
          reversible={block.reversible}
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

    // CustomContentBlockMap kinds (third-party + preview-blocks) only.
    default:
      return <PluginContentBlock key={key} block={block} />;
  }
}

export function renderMessageBlocks(message: Message, ctx: BlockCtx) {
  return messageBlockRenderUnits(message.blocks, ctx.toolCalls).map((unit) => {
    if (unit.kind === "toolGroup") {
      return (
        <ToolGroup
          key={`group-${unit.tools[0]!.id}`}
          tools={unit.tools}
          onSelectTool={ctx.onSelectTool}
          expandedIds={ctx.expandedIds}
          onToggleExpand={ctx.onToggleExpand}
        />
      );
    }
    return renderBlock(unit.block, unit.index, ctx);
  });
}
