import type { BlockCtx } from "./BlockRenderer";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { memo, useMemo } from "react";
import { ToolGroup } from "@/plugins/builtin/chat/tools/public/rendering";
import { useCitationSources } from "@/plugins/sdk";
import { Slot } from "@/plugins/host/Slot";
import { MessageContext } from "@/plugins/sdk/messageContext";
import {
  messageActionsVisibility,
  messageActionsVisibilityClass,
} from "@/plugins/builtin/chat/message-actions/public/messageActions";
import {
  messageBlockRenderUnits,
  messageBlocksRenderInstant,
  messageCitations,
} from "../application/messageBlockModel";
import { cn } from "@/lib/utils";
import { CitationContext } from "./CitationContext";
import { MessageContextMenu } from "./MessageContextMenu";
import { renderBlock } from "./BlockRenderer";

function MessageBlockInner({
  msg,
  ctx,
  isLast,
  isRunning,
}: {
  msg: Message;
  ctx: BlockCtx;
  /** Last turn in the thread — its action bar stays pinned open. */
  isLast: boolean;
  /** A run is streaming — action bars stay hidden until it settles.
   *  Flips only at run boundaries, so it never churns this memo per token. */
  isRunning: boolean;
}) {
  const isUser = msg.role === "user";

  const sources = useCitationSources();
  const citations = useMemo(() => messageCitations(msg.blocks, sources), [msg.blocks, sources]);

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

  const blockCtx: BlockCtx = messageBlocksRenderInstant(msg.role) ? { ...ctx, instant: true } : ctx;

  const content = messageBlockRenderUnits(msg.blocks, blockCtx.toolCalls).map((unit) => {
    if (unit.kind === "toolGroup") {
      return (
        <ToolGroup
          key={`group-${unit.tools[0]!.id}`}
          tools={unit.tools}
          onSelectTool={blockCtx.onSelectTool}
          expandedIds={blockCtx.expandedIds}
          onToggleExpand={blockCtx.onToggleExpand}
        />
      );
    }
    const { block, index } = unit;
    return renderBlock(block, index, blockCtx);
  });

  const actionsClass = cn(
    "mt-1 flex transition-opacity duration-[--dur-fast]",
    messageActionsVisibilityClass(messageActionsVisibility({ isRunning, isLast })),
  );

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
                <div className="msg-content min-w-0 max-w-[80%] rounded-[20px] bg-surface-2 px-4 py-2.5 text-left text-[15px] leading-[1.5] text-fg">
                  {content}
                </div>
              </MessageContextMenu>
              {/* Action bar — icon-only, rounded-full to match the bubble
                  language. Visibility follows the state machine above. */}
              <div className={actionsClass} data-slot="message-actions">
                <Slot name="message.actions" />
              </div>
            </div>
          ) : (
            <div className="group flex" data-slot="message-assistant">
              <div className="min-w-0 flex-1">
                <MessageContextMenu msg={msg}>
                  <div className="msg-content max-w-[var(--content-max)] text-pretty text-[15px] leading-[1.7] text-fg-soft">
                    {content}
                  </div>
                </MessageContextMenu>
                {/* Action bar — icon-only, rounded-md for quieter assistant
                    chrome. Visibility follows the state machine above. */}
                <div className={actionsClass} data-slot="message-actions">
                  <Slot name="message.actions" />
                </div>
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
