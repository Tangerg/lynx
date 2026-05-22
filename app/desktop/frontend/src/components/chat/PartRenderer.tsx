import type { ContentBlock, PlanItem, ToolCall } from "@/protocol/agui/viewState";
import { MarkdownMessage } from "@/components/chat/MarkdownMessage";
import { ToolCard } from "@/components/tools/ToolCard";
import { PluginContentBlock } from "@/plugins/PluginContentBlock";
import { openViewForTool } from "@/state/toolRouting";

/**
 * Per-render bag of data threaded into block renderers. Kept narrow —
 * UI-state knobs (selected tool, expanded set, plan) flow through here.
 * The "open the full view" action lives in `openViewForTool` so the
 * callback doesn't have to be threaded down.
 */
export type PartCtx = {
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
};

/**
 * Render one content block.
 *
 * Only `text` and `tool` are handled in core. `text` is the primary message
 * stream and `tool` is a registry indirection (the ToolCard itself routes
 * the inline preview through the plugin tool-preview registry). Everything
 * else — plan / code / search / approval / checkpoint / reasoning — is
 * rendered by plugin-contributed components via PluginContentBlock.
 *
 * Plugin-contributed kinds use the exact same path, since they're declared
 * via `CustomContentBlockMap` and their renderers go in the same registry.
 */
export function renderPart(block: ContentBlock, key: number, ctx: PartCtx) {
  switch (block.kind) {
    case "text":
      return (
        <p key={key} className={block.streaming && !ctx.instant ? "streaming" : undefined}>
          <MarkdownMessage text={block.text} streaming={block.streaming} instant={ctx.instant} />
        </p>
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

    default:
      return <PluginContentBlock key={key} block={block} />;
  }
}
