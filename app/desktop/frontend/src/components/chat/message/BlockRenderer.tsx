// Message block dispatcher — maps each ContentBlock (text, tool, reasoning,
// approval, question, plan, compaction, search, code, checkpoint) to its React
// card. Kept as a lookup table (BLOCK_RENDERERS) so adding a new block kind is
// one row — no if/elif ladder growing with the protocol.
import type { ContentBlock, PlanItem, ToolCall } from "@/protocol/run/viewState";
import { isQuestionTool } from "@/protocol/run/viewState";
import { MarkdownMessage } from "./markdown/MarkdownMessage";
import {
  ApprovalCard,
  CompactionBlock,
  ImageBlock,
  PlanBlock,
  QuestionCard,
  ReasoningBlock,
} from "./cards";
import { ToolCard } from "@/components/tools/ToolCard";
import { isReadOnlyTool } from "@/components/tools/ToolGroup";
import { PluginContentBlock } from "@/plugins/host/PluginContentBlock";
import { hasToolView, openViewForTool } from "@/state/toolRouting";

/**
 * A unit of rendering: either a single content block (with its ORIGINAL index,
 * which the caller's streaming-text coercion keys off) or a folded run of
 * adjacent read-only tool calls.
 */
type RenderUnit =
	| { kind: "block"; block: ContentBlock; index: number }
	| { kind: "toolGroup"; tools: ToolCall[] };

/**
 * Fold a message's blocks into render units, collapsing runs of 2+ adjacent
 * read-only tool calls (`read` / `grep` / `glob` / `lsp` / `lsp_diagnostics`) into one
 * `toolGroup` so a long agent turn stays scannable instead of flooding the
 * transcript with a card per read. A lone read-only call, or any
 * side-effecting tool, stays its own block and renders as a normal card. Pure
 * and index-preserving — the caller still keys streaming-text coercion off the
 * original block index.
 */
export function planRenderUnits(
  blocks: ContentBlock[],
  toolCalls: Record<string, ToolCall>,
): RenderUnit[] {
  const units: RenderUnit[] = [];
  // A HITL-question tool's toolCall row is the redundant shadow of its question
  // block (see isQuestionTool) — drop it, but only when that question block is
  // actually present, so a non-parking runtime that returns ask_user as a plain
  // tool result still renders its card.
  const hasQuestion = blocks.some((b) => b.kind === "question");
  let run: { block: ContentBlock; index: number; tool: ToolCall }[] = [];
  const flush = () => {
    if (run.length >= 2) {
      units.push({ kind: "toolGroup", tools: run.map((r) => r.tool) });
    } else {
      for (const r of run) units.push({ kind: "block", block: r.block, index: r.index });
    }
    run = [];
  };
  blocks.forEach((block, index) => {
    if (block.kind === "tool") {
      const tool = toolCalls[block.toolCallId];
      if (tool && isReadOnlyTool(tool.name)) {
        run.push({ block, index, tool });
        return;
      }
      if (tool && hasQuestion && isQuestionTool(tool.name)) {
        flush(); // shadow of the question block — render nothing, break the run
        return;
      }
    }
    flush(); // a non-grouped block breaks the run
    units.push({ kind: "block", block, index });
  });
  flush();
  return units;
}

/**
 * Per-render bag of data threaded into block renderers. Kept narrow —
 * UI-state knobs (selected tool, expanded set, plan) flow through here.
 * The "open the full view" action lives in `openViewForTool` so the
 * callback doesn't have to be threaded down.
 */
export interface BlockCtx {
  plan: PlanItem[];
  toolCalls: Record<string, ToolCall>;
  selectedToolId: string;
  onSelectTool: (id: string) => void;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  /**
   * Skip stream-smoothing and the fade-in animation for this message.
   * Used for user-typed messages — the author already saw the text they
   * typed, so animating it back at them feels patronizing and slow.
   */
  instant?: boolean;
  /**
   * Reveal streamed assistant text character-by-character (typewriter) instead
   * of word-by-word with a fade (smooth). Global preference, read once in
   * ChatStream so it doesn't re-subscribe per message block.
   */
  typewriter?: boolean;
}

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
      return (
        <ToolCard
          // Identity key, NOT the block index — same reasoning as the HITL
          // cards below: ToolCard owns an expand animation + selection, and a
          // stable per-tool key keeps React from reusing one card's instance
          // for a different tool should block order ever shift.
          key={block.toolCallId}
          tool={tool}
          selected={ctx.selectedToolId === block.toolCallId}
          expanded={ctx.expandedIds.has(block.toolCallId)}
          onToggleExpand={() => {
            ctx.onSelectTool(block.toolCallId);
            ctx.onToggleExpand(block.toolCallId);
          }}
          onOpenView={hasToolView(tool) ? () => openViewForTool(block.toolCallId) : undefined}
        />
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
          what={block.text}
          cmd={block.command}
          reason={block.reason}
          parentRunId={block.parentRunId}
          itemId={block.itemId}
          decision={block.decision}
          args={block.args}
          risk={block.risk}
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
          parentRunId={block.parentRunId}
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
