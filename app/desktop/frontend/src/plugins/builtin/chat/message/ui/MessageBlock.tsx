import type { BlockCtx } from "./BlockRenderer";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { memo, useMemo } from "react";
import { useCitationSources } from "@/plugins/sdk";
import { Slot } from "@/plugins/host/Slot";
import { MessageContext } from "@/plugins/sdk/messageContext";
import {
  messageActionsVisibility,
  type MessageActionsVisibility,
} from "@/plugins/builtin/chat/message-actions/public/messageActions";
import { messageBlocksRenderInstant, messageCitations } from "../application/messageBlockModel";
import { cn } from "@/lib/classNames";
import { MESSAGE_CONTENT_CLASS } from "./messageContent";
import { CitationContext } from "./CitationContext";
import { MessageContextMenu } from "./MessageContextMenu";
import { renderBlock, renderMessageBlocks } from "./BlockRenderer";

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
        <div className={MESSAGE_CONTENT_CLASS}>
          {msg.blocks.map((block, index) => renderBlock(block, index, ctx))}
        </div>
      </MessageContext.Provider>
    );
  }

  const blockCtx: BlockCtx = messageBlocksRenderInstant(msg.role) ? { ...ctx, instant: true } : ctx;

  const content = renderMessageBlocks(msg, blockCtx);

  const actionsClass = cn(
    "mt-1 flex transition-opacity duration-[--dur-fast]",
    ACTIONS_VISIBILITY[messageActionsVisibility({ isRunning, isLast })],
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
            <div className="group flex flex-col items-end">
              <MessageContextMenu msg={msg}>
                <div
                  className={cn(
                    MESSAGE_CONTENT_CLASS,
                    "min-w-0 max-w-[80%] rounded-bubble bg-control px-4 py-2.5 text-left text-ui-lg leading-relaxed text-fg",
                  )}
                >
                  {content}
                </div>
              </MessageContextMenu>
              {/* Action bar — icon-only, rounded-full to match the bubble
                  language. Visibility follows the state machine above. */}
              <div className={actionsClass}>
                <Slot name="message.actions" />
              </div>
            </div>
          ) : (
            <div className="group flex">
              <div className="min-w-0 flex-1">
                <MessageContextMenu msg={msg}>
                  <div
                    className={cn(
                      MESSAGE_CONTENT_CLASS,
                      "max-w-[var(--content-max)] text-pretty text-ui-md leading-relaxed text-fg-soft",
                    )}
                  >
                    {content}
                  </div>
                </MessageContextMenu>
                {/* Action bar — icon-only, rounded-md for quieter assistant
                    chrome. Visibility follows the state machine above. */}
                <div className={actionsClass}>
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

// How each action-bar visibility state looks. The state machine is the
// message-actions context's rule; what it looks like is this view's, and this is
// the only view that renders the bar. Hover reveal stays in CSS (`group-hover` /
// `focus-within`) rather than JS so a hovering pointer never triggers a render;
// the ancestor carrying `.group` is the message container below.
const ACTIONS_VISIBILITY: Record<MessageActionsVisibility, string> = {
  hidden: "pointer-events-none opacity-0",
  hover: "opacity-0 group-hover:opacity-100 focus-within:opacity-100",
  pinned: "opacity-100",
};
