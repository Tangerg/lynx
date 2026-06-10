// MessageBlock — one chat turn.
//
// Layout: avatar + name + time on one header row, then the content
// on a new line aligned with the avatar's left edge (Cherry Studio
// pattern). User-bubble mode flips the header to right-aligned and
// drops a chat bubble below; user-plain and assistant share the
// left-aligned flat-prose layout.

import type { Citation } from "./CitationContext";
import type { BlockCtx } from "./BlockRenderer";
import type { IconName } from "@/components/common";
import type { Message } from "@/protocol/run/viewState";
import { memo, useMemo, useRef } from "react";
import { Icon } from "@/components/common";
import { Avatar } from "@/components/common/Avatar";
import { cn } from "@/lib/utils";
import { MESSAGE_ROLE, useCitationSources, useExtensionByKey } from "@/plugins/sdk";
import { Slot } from "@/plugins/host/Slot";
import { useUiStore } from "@/state/uiStore";
import { MessageContext } from "@/plugins/sdk/messageContext";
import { CitationContext } from "./CitationContext";
import { MessageContextMenu } from "./MessageContextMenu";
import { MessageOutline } from "./MessageOutline";
import { renderBlock } from "./BlockRenderer";

function MessageBlockInner({
  msg,
  ctx,
  assistantName,
}: {
  msg: Message;
  ctx: BlockCtx;
  /** Live model name for assistant turns (resolved once in ChatStream from the
   *  session's model). Falls back to the role's neutral displayName. */
  assistantName?: string;
}) {
  const role = useExtensionByKey(MESSAGE_ROLE, msg.role);
  const isUser = msg.role === "user";
  const isAgent = msg.role === "assistant";
  // Bubble (right-aligned card) is the default for user messages.
  // Plain mirrors the assistant layout — left-aligned flat prose, no card.
  const bubble = useUiStore((s) => s.messageStyle) === "bubble" && isUser;

  // Outline target — only mounted for assistant messages. Bubble user
  // messages are short and don't need a TOC.
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

  const displayName = (isAgent && assistantName) || role?.displayName || msg.who;
  const avatarVariant = (role?.avatarVariant ?? (isUser ? "msg-user" : "msg-agent")) as
    | "msg-user"
    | "msg-agent";
  const iconName = (role?.icon ?? (isUser ? "user" : "spark")) as IconName;

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

  return (
    <MessageContext.Provider value={msg}>
      <CitationContext.Provider value={citations}>
        {/* minmax(0,1fr) caps the implicit grid column at the parent's
            width — without it, a wide child (e.g. a ReasoningBlock with
            a long preview line) stretches the whole row past the
            intended msg-stream column. */}
        <div className="relative grid grid-cols-[minmax(0,1fr)] gap-2">
          {/* Header: avatar paired with a vertical (title / time) stack.
              User-bubble flips the row so the avatar lands on the right
              and the stack right-aligns its rows. */}
          <div className={cn("flex items-center gap-2.5", bubble && "flex-row-reverse")}>
            <Avatar variant={avatarVariant}>
              <Icon name={iconName} size={14} />
            </Avatar>
            <div
              className={cn("flex min-w-0 flex-col gap-0.5 leading-tight", bubble && "items-end")}
            >
              <div
                className={cn(
                  "flex items-center gap-1.5 text-fg text-[13px] font-semibold tracking-normal",
                  isAgent && "font-mono",
                )}
              >
                <span className="truncate">{displayName}</span>
                <Slot name="message.header.end" />
              </div>
              <span className="font-mono text-[11px] text-fg-faint">{msg.time}</span>
            </div>
          </div>

          {/* Content row. Plain layouts (agent + user-plain) start at
              the row's left edge so they line up with the avatar above.
              User-bubble floats a max-width card to the right so its
              right edge matches the header's right edge. The whole
              content surface is the right-click target — the inline
              header icons are also there for hover discovery, but the
              context menu is the platform-native discovery path. */}
          <MessageContextMenu msg={msg}>
            <div className={cn(bubble && "flex justify-end")}>
              <div
                ref={contentRef}
                className={cn(
                  // 15px is the content baseline — markdown headings and
                  // every other content surface size off this.
                  "msg-content min-w-0 text-fg text-[15px] leading-[1.68] tracking-[-0.003em] font-normal",
                  bubble &&
                    "max-w-[580px] rounded-[14px_14px_4px_14px] bg-surface-2 px-3.5 py-2.5 text-left light:bg-surface-3",
                )}
              >
                {msg.blocks.map((block, i) => {
                  if (block.kind === "text" && block.status === "running" && i !== lastIdx) {
                    return renderBlock({ ...block, status: "complete" }, i, blockCtx);
                  }
                  return renderBlock(block, i, blockCtx);
                })}
              </div>
            </div>
          </MessageContextMenu>

          <div className={cn(bubble && "flex justify-end")}>
            <Slot name="message.actions" />
          </div>

          {/* Right-gutter outline. Hidden on narrow viewports where no
              gutter is available. Skipped while *any* block on the message
              is still streaming — the outline is a "jump to a finished
              heading" affordance, and the per-token MutationObserver
              rebuild used to compete with use-stick-to-bottom and cause
              the chat to snap back during streaming. */}
          {isAgent && !isStreaming && <MessageOutline target={contentRef} />}
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
