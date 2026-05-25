// MessageBlock — one chat turn. Role identity comes from the plugin
// registry. Agent rows take full-width prose with avatar to the left;
// user rows are right-aligned bubbles with avatar to the right.

import type {Citation} from "./CitationContext";
import type {PartCtx} from "./PartRenderer";
import type {IconName} from "@/components/common";
import type { Message } from "@/protocol/agui/viewState";
import { useRef } from "react";
import { Icon  } from "@/components/common";
import { Avatar } from "@/components/common/Avatar";
import { cn } from "@/lib/utils";
import { useMessageRole } from "@/plugins/sdk";
import { Slot } from "@/plugins/Slot";
import { useThemeStore } from "@/state/themeStore";
import {  CitationContext } from "./CitationContext";
import { MessageContext } from "./MessageContext";
import { MessageOutline } from "./MessageOutline";
import {  renderPart } from "./PartRenderer";

export function MessageBlock({ msg, ctx }: { msg: Message; ctx: PartCtx }) {
  const role = useMessageRole(msg.role);
  const isUser = msg.role === "user";
  const isAgent = msg.role === "assistant";
  // Bubble (right-aligned card) is the default for user messages.
  // Plain mirrors the assistant layout — left-aligned flat prose, no card.
  const bubble = useThemeStore((s) => s.messageStyle) === "bubble" && isUser;

  // Outline target — only mounted for assistant messages. Bubble user
  // messages are short and don't need a TOC.
  const contentRef = useRef<HTMLDivElement>(null);

  // Citation registry — flatten every `search` block on this message
  // into a 1-indexed list keyed by `[n]` markers in the prose. The
  // CitationBadge component reads this via context.
  const citations: Citation[] = [];
  for (const b of msg.blocks) {
    if (b.kind !== "search") continue;
    for (const r of b.results) {
      citations.push({
        index: citations.length + 1,
        domain: r.domain,
        title: r.title,
        snippet: r.snippet,
      });
    }
  }

  const displayName = role?.displayName ?? msg.who;
  const avatarVariant = (role?.avatarVariant ?? (isUser ? "msg-user" : "msg-agent")) as
    | "msg-user"
    | "msg-agent";
  const iconName = (role?.icon ?? (isUser ? "user" : "spark")) as IconName;

  // Only the last text block keeps `streaming: true` so we don't draw
  // a caret at the end of every intermediate text block (the reducer
  // leaves them all streaming until TEXT_MESSAGE_END).
  const lastIdx = msg.blocks.length - 1;

  // Skip the smooth-text + fade-in pipeline for user messages — they
  // already saw what they typed; replaying it adds latency for no gain.
  const partCtx: PartCtx = isUser ? { ...ctx, instant: true } : ctx;

  return (
    <MessageContext.Provider value={msg}>
     <CitationContext.Provider value={citations}>
      <div
        className={cn(
          "relative grid items-start gap-1",
          isAgent && "grid-cols-[1fr] pl-[46px]",
          // Bubble mode: avatar on the right, right-aligned content.
          // Plain mode: same as assistant — avatar on the left.
          isUser && (bubble ? "grid-cols-[1fr] pr-[46px]" : "grid-cols-[1fr] pl-[46px]"),
          !isAgent && !isUser && "grid-cols-[32px_1fr] gap-3.5",
        )}
      >
        <div
          className={cn(
            "shrink-0",
            (isAgent || isUser) && "absolute top-0.5",
            isAgent && "left-0",
            isUser && (bubble ? "right-0" : "left-0"),
          )}
        >
          <Avatar variant={avatarVariant}>
            <Icon name={iconName} size={14} />
          </Avatar>
        </div>
        <div className={cn("min-w-0", bubble && "flex flex-col items-end")}>
          <div
            className={cn(
              "mb-1 flex items-center gap-2 whitespace-nowrap text-[12px] text-fg-faint",
              bubble && "justify-end",
            )}
          >
            <span
              className={cn(
                "text-fg text-[13px] font-semibold tracking-normal",
                isAgent && "font-mono",
              )}
            >
              {displayName}
            </span>
            <span>·</span>
            <span>{msg.time}</span>
            <Slot name="message.header.end" />
          </div>
          <div className={cn(isAgent && "flex items-start gap-3")}>
            <div
              ref={contentRef}
              className={cn(
                // 15px is the content baseline — markdown headings and
                // every other content surface size off this.
                "msg-content min-w-0 flex-1 text-fg text-[15px] leading-[1.68] tracking-[-0.003em] font-normal",
                bubble &&
                  "max-w-[580px] rounded-[14px_14px_4px_14px] bg-surface-2 px-3.5 py-2.5 text-left light:bg-surface-3",
              )}
            >
              {msg.blocks.map((part, i) => {
                if (part.kind === "text" && part.streaming && i !== lastIdx) {
                  return renderPart({ ...part, streaming: false }, i, partCtx);
                }
                return renderPart(part, i, partCtx);
              })}
            </div>
            {isAgent && <MessageOutline target={contentRef} />}
          </div>
          <Slot name="message.actions" />
        </div>
      </div>
     </CitationContext.Provider>
    </MessageContext.Provider>
  );
}
