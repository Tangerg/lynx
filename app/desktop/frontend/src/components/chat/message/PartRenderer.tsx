import type { ContentBlock, PlanItem, ToolCall } from "@/protocol/run/viewState";
import { MarkdownMessage } from "./markdown/MarkdownMessage";
import { ApprovalCard, PlanBlock, QuestionCard, ReasoningBlock } from "./cards";
import { ToolCard } from "@/components/tools/ToolCard";
import { PluginContentBlock } from "@/plugins/host/PluginContentBlock";
import { openViewForTool } from "@/state/toolRouting";

/**
 * Per-render bag of data threaded into block renderers. Kept narrow —
 * UI-state knobs (selected tool, expanded set, plan) flow through here.
 * The "open the full view" action lives in `openViewForTool` so the
 * callback doesn't have to be threaded down.
 */
export interface PartCtx {
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
export function renderPart(block: ContentBlock, key: number, ctx: PartCtx) {
  switch (block.kind) {
    case "text":
      // Wrapper is a <div>, not a <p>: react-markdown emits <p> nodes
      // of its own, and `<p>` inside `<p>` is invalid HTML (browsers
      // silently split the outer one). The earlier <p> wrapper also
      // triggered tokens.css's naked-element `p { font-size: var(
      // --fs-body-md) }` (14.08px) — which then propagated through
      // `.md` and made every chat message render at 14px regardless of
      // the Tailwind `text-[16px]` set on .msg-content. The `streaming`
      // class was a dead marker (no CSS rule referenced it) so it goes
      // away with the wrapper.
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

    case "tool": {
      const tool = ctx.toolCalls[block.toolCallId];
      if (!tool) return null;
      return (
        <ToolCard
          key={key}
          tool={tool}
          selected={ctx.selectedToolId === block.toolCallId}
          expanded={ctx.expandedIds.has(block.toolCallId)}
          onToggleExpand={() => {
            ctx.onSelectTool(block.toolCallId);
            ctx.onToggleExpand(block.toolCallId);
          }}
          onOpenView={() => openViewForTool(block.toolCallId)}
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
      return (
        <ApprovalCard
          key={key}
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
      return (
        <QuestionCard
          key={key}
          status={block.status}
          parentRunId={block.parentRunId}
          itemId={block.itemId}
          questions={block.questions}
          answered={block.answered}
        />
      );

    // CustomContentBlockMap kinds (third-party + preview-blocks) only.
    default:
      return <PluginContentBlock key={key} block={block} />;
  }
}
