import type { ContentBlock, PlanItem, ToolCall } from "@/protocol/agui/viewState";
import { ToolCard } from "@/components/tools/ToolCard";
import { PluginContentBlock } from "@/plugins/PluginContentBlock";
import { renderInline } from "@/utils/inline";

export type PartCtx = {
  plan: PlanItem[];
  toolCalls: Record<string, ToolCall>;
  selectedToolId: string;
  onSelectTool: (id: string) => void;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  onOpenInspector: (id: string) => void;
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
        <p
          key={key}
          className={block.streaming ? "streaming" : undefined}
          dangerouslySetInnerHTML={{
            __html: renderInline(block.text) + (block.streaming ? '<span class="cursor">▌</span>' : ""),
          }}
        />
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
          onOpenInspector={() => ctx.onOpenInspector(block.toolCallId)}
        />
      );
    }

    default:
      return <PluginContentBlock key={key} block={block} />;
  }
}
