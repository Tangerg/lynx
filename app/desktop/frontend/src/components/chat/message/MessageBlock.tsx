// MessageBlock — one chat turn.
//
// Craft-aligned asymmetric layout:
//   • User = right-aligned gray bubble (input), no avatar/header chrome.
//   • Assistant = document surface (ResponseCard-style), no per-message header;
//     prose + inline tool rows sit inside a subtle lifted surface.

import type { Citation } from "./CitationContext";
import type { BlockCtx } from "./BlockRenderer";
import type { Message } from "@/protocol/run/viewState";
import { memo, useMemo, useRef } from "react";
import { ToolGroup } from "@/components/tools/ToolGroup";
import { useCitationSources } from "@/plugins/sdk";
import { Slot } from "@/plugins/host/Slot";
import { MessageContext } from "@/plugins/sdk/messageContext";
import { CitationContext } from "./CitationContext";
import { MessageContextMenu } from "./MessageContextMenu";
import { MessageOutline } from "./MessageOutline";
import { planRenderUnits, renderBlock } from "./BlockRenderer";

function MessageBlockInner({ msg, ctx }: { msg: Message; ctx: BlockCtx }) {
  const isUser = msg.role === "user";
  const isAgent = msg.role === "assistant";

  // Outline target — only consumed by assistant messages (MessageOutline).
  const contentRef = useRef<HTMLDivElement>(null);

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
        <div className="msg-content">
          {msg.blocks.map((block, index) => renderBlock(block, index, ctx))}
        </div>
      </MessageContext.Provider>
    );
  }

  // Only the last text block keeps `status === "running"` so we don't draw
  // a caret at the end of every intermediate text block (the reducer
  // leaves them all running until TEXT_MESSAGE_END).
  const lastIdx = msg.blocks.length - 1;

  // True while any block on this message is still streaming. Gates the
  // MessageOutline mount so the per-token MutationObserver inside doesn't
  // fire while content is in motion.
  const isStreaming = msg.blocks.some(
    (b) => (b.kind === "text" || b.kind === "reasoning") && b.status === "running",
  );

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
            <>
              {/* User = right-aligned gray bubble; no avatar, no header. */}
              <MessageContextMenu msg={msg}>
                <div className="flex justify-end">
                  <div
                    ref={contentRef}
                    className="msg-content min-w-0 max-w-[80%] rounded-2xl bg-surface-2 px-5 py-3.5 text-left light:bg-surface-3 text-fg text-[15px] leading-[1.68] tracking-[-0.003em] font-normal"
                  >
                    {content}
                  </div>
                </div>
              </MessageContextMenu>
              <div className="flex justify-end">
                <Slot name="message.actions" />
              </div>
            </>
          ) : (
            <>
              {/* Assistant = document surface (craft-style ResponseCard).
                  No per-message header chrome; prose + inline tool rows sit
                  inside a subtle lifted surface so they read as a generated
                  document, not chat lines on the canvas. Actions surface only
                  on hover. */}
              <MessageContextMenu msg={msg}>
                <div
                  ref={contentRef}
                  className="msg-content group/msg min-w-0 rounded-md bg-surface shadow-minimal px-5 py-4 text-fg text-[15px] leading-[1.68] tracking-[-0.003em] font-normal"
                >
                  {content}
                  {/* Hover-only action bar — appears when the user hovers the
                      document surface. Kept inside the surface so it doesn't
                      float over adjacent messages. */}
                  <div className="mt-2 flex justify-end opacity-0 transition-opacity duration-150 group-hover/msg:opacity-100">
                    <Slot name="message.actions" />
                  </div>
                </div>
              </MessageContextMenu>

              {/* Right-gutter outline. Hidden on narrow viewports where no
                  gutter is available. Skipped while *any* block on the message
                  is still streaming — the outline is a "jump to a finished
                  heading" affordance, and the per-token DOM mutations from
                  streaming compete with use-stick-to-bottom, causing the
                  chat to snap back. */}
              {isAgent && !isStreaming && <MessageOutline target={contentRef} scopeId={msg.id} />}
            </>
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
