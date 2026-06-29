// MessageBlock — one chat turn.
//
// OpenAI-restrained asymmetric layout:
//   • User = right-aligned bubble (input), no avatar/header chrome.
//   • Assistant = unboxed prose directly on the canvas, no surface / header.

import type { Citation } from "./CitationContext";
import type { BlockCtx } from "./BlockRenderer";
import type { Message } from "@/protocol/run/viewState";
import { memo, useMemo } from "react";
import { ToolGroup } from "@/components/tools/ToolGroup";
import { useCitationSources } from "@/plugins/sdk";
import { Slot } from "@/plugins/host/Slot";
import { MessageContext } from "@/plugins/sdk/messageContext";
import { CitationContext } from "./CitationContext";
import { MessageContextMenu } from "./MessageContextMenu";
import { planRenderUnits, renderBlock } from "./BlockRenderer";

function MessageBlockInner({ msg, ctx }: { msg: Message; ctx: BlockCtx }) {
  const isUser = msg.role === "user";

  // Citation registry — gathered from the MESSAGE_CITATION_SOURCE
  // contributions (the search-block plugin maps its results in; with no such
  // plugin the list is empty and `[n]` markers render as plain text). The
  // kernel owns the 1-indexed continuity across sources. CitationBadge reads
  // this via context. Memoised on msg.blocks + sources so the array identity
  // stays stable across re-renders that don't touch citation content — keeps
  // `<CitationContext.Provider value={citations}>` from churning every render
  // and re-triggering all CitationBadge consumers downstream.
  const sources = useCitationSources();
  const citations = useMemo<Citation[]>(
    () => sources.flatMap((s) => s(msg.blocks)).map((c, i) => ({ ...c, index: i + 1 })),
    [msg.blocks, sources],
  );

  // System messages (e.g. a compaction boundary) are chrome-less full-width
  // notes — no avatar / name / time / outline / context-menu, just the block(s)
  // rendered inline (CompactionBlock draws its own divider). Placed after all
  // hooks so rules-of-hooks holds.
  if (msg.role === "system") {
    return (
      <MessageContext.Provider value={msg}>
        <div className="msg-content" data-slot="message-system">
          {msg.blocks.map((block, index) => renderBlock(block, index, ctx))}
        </div>
      </MessageContext.Provider>
    );
  }

  // Only the last text block keeps `status === "running"` so we don't draw
  // a caret at the end of every intermediate text block (the reducer
  // leaves them all running until TEXT_MESSAGE_END).
  const lastIdx = msg.blocks.length - 1;

  // Skip the stream-reveal + fade-in pipeline for user messages — they
  // already saw what they typed; replaying it adds latency for no gain.
  const blockCtx: BlockCtx = isUser ? { ...ctx, instant: true } : ctx;

  const content = planRenderUnits(msg.blocks, blockCtx.toolCalls).map((unit) => {
    if (unit.kind === "toolGroup") {
      return (
        <ToolGroup
          key={`group-${unit.tools[0]!.id}`}
          tools={unit.tools}
          selectedToolId={blockCtx.selectedToolId}
          onSelectTool={blockCtx.onSelectTool}
          expandedIds={blockCtx.expandedIds}
          onToggleExpand={blockCtx.onToggleExpand}
        />
      );
    }
    const { block, index } = unit;
    if (block.kind === "text" && block.status === "running" && index !== lastIdx) {
      return renderBlock({ ...block, status: "complete" }, index, blockCtx);
    }
    return renderBlock(block, index, blockCtx);
  });

  return (
    <MessageContext.Provider value={msg}>
      <CitationContext.Provider value={citations}>
        {/* minmax(0,1fr) caps the implicit grid column at the parent's
            width — without it, a wide child (e.g. a ReasoningBlock with
            a long preview line) stretches the whole row past the
            intended msg-stream column. */}
        <div className="relative grid grid-cols-[minmax(0,1fr)] gap-1.5">
          {isUser ? (
            <div className="group flex flex-col items-end" data-slot="message-user">
              <MessageContextMenu msg={msg}>
                <div className="msg-content min-w-0 max-w-[80%] rounded-xl bg-surface-2 px-4 py-2.5 text-left text-fg text-[15px] leading-relaxed">
                  {content}
                </div>
              </MessageContextMenu>
              {/* Hover-reveal action bar — icon-only, rounded-full to match
                  the bubble language. */}
              <div
                className="mt-1 flex opacity-0 transition-opacity duration-[--dur-fast] group-hover:opacity-100 focus-within:opacity-100"
                data-slot="message-actions"
              >
                <Slot name="message.actions" />
              </div>
            </div>
          ) : (
            <div className="group" data-slot="message-assistant">
              <MessageContextMenu msg={msg}>
                <div className="msg-content min-w-0 max-w-[--content-max] text-fg-soft text-[15px] leading-relaxed">
                  {content}
                </div>
              </MessageContextMenu>
              {/* Hover-reveal action bar — icon-only, rounded-md for quieter
                  assistant chrome. */}
              <div
                className="mt-1 flex opacity-0 transition-opacity duration-[--dur-fast] group-hover:opacity-100 focus-within:opacity-100"
                data-slot="message-actions"
              >
                <Slot name="message.actions" />
              </div>
            </div>
          )}
        </div>
      </CitationContext.Provider>
    </MessageContext.Provider>
  );
}

// React.memo with default shallow comparison. The reducer's
// updateMessage keeps non-modified messages at the same reference, so
// during pure text streaming only the tail message's `msg` prop ref
// changes — every other MessageBlock bails out of the render path
// (with 200 messages on screen this was 199× redundant work per token
// delta). ctx identity is stabilised in ChatStream via useMemo so
// non-tool / non-plan churn doesn't invalidate this memo either.
export const MessageBlock = memo(MessageBlockInner);
