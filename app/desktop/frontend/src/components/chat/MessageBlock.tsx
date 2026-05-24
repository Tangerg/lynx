// MessageBlock — renders one chat turn.
//
// Role identity (display name + avatar icon) is looked up from the
// plugin registry. Fallbacks render a generic identity when no plugin
// has registered the role.
//
// Layout:
//   - Agent / system messages: full-width body, avatar absolute-positioned
//     46px to the left so the body keeps its measure.
//   - User messages: right-aligned bubble, avatar absolute on the right.
//
// `msg-content` className is preserved (not Tailwind-only) — markdown
// content styling lives there and the MarkdownMessage component nests
// many elements (p, code, table, etc.) inside it.

import { Icon, type IconName } from "@/components/common";
import { Avatar } from "@/components/common/Avatar";
import { cn } from "@/lib/utils";
import { Slot } from "@/plugins/Slot";
import { useMessageRole } from "@/plugins/sdk";
import type { Message } from "@/protocol/agui/viewState";
import { MessageContext } from "./MessageContext";
import { renderPart, type PartCtx } from "./PartRenderer";

export function MessageBlock({ msg, ctx }: { msg: Message; ctx: PartCtx }) {
  const role = useMessageRole(msg.role);
  const isUser = msg.role === "user";
  const isAgent = msg.role === "assistant";

  const displayName = role?.displayName ?? msg.who;
  const avatarVariant =
    (role?.avatarVariant ?? (isUser ? "msg-user" : "msg-agent")) as "msg-user" | "msg-agent";
  const iconName = (role?.icon ?? (isUser ? "user" : "spark")) as IconName;

  // The streaming-tail caret should only sit at the END of the message —
  // not at the end of every text block. When an assistant message
  // interleaves text + tool calls + more text, the reducer leaves every
  // intermediate text block with `streaming: true` until TEXT_MESSAGE_END
  // closes them all at once. Without this gating each older text block
  // would render its own cursor and leave "ghost carets" stranded mid-bubble.
  const lastIdx = msg.blocks.length - 1;

  // The user already saw what they typed — replaying their own message
  // through the smooth-text + fade-in pipeline feels patronizing and
  // adds a couple of seconds of latency before they can read it back.
  const partCtx: PartCtx = isUser ? { ...ctx, instant: true } : ctx;

  return (
    <MessageContext.Provider value={msg}>
      <div
        className={cn(
          "relative grid items-start gap-1",
          isAgent && "grid-cols-[1fr] pl-[46px]",
          isUser && "grid-cols-[1fr] pr-[46px]",
          !isAgent && !isUser && "grid-cols-[32px_1fr] gap-3.5",
        )}
      >
        <div
          className={cn(
            "shrink-0",
            (isAgent || isUser) && "absolute top-0.5",
            isAgent && "left-0",
            isUser && "right-0",
          )}
        >
          <Avatar variant={avatarVariant}>
            <Icon name={iconName} size={14} />
          </Avatar>
        </div>
        <div
          className={cn(
            "min-w-0",
            isUser && "flex flex-col items-end",
          )}
        >
          <div
            className={cn(
              "mb-1 flex items-center gap-2 whitespace-nowrap text-[10.5px] text-fg-faint opacity-65",
              isUser && "justify-end",
            )}
          >
            <span
              className={cn(
                "text-fg text-[12px] font-semibold tracking-normal",
                isAgent && "font-mono",
              )}
            >
              {displayName}
            </span>
            <span>·</span>
            <span>{msg.time}</span>
            <Slot name="message.header.end" />
          </div>
          <div
            className={cn(
              "msg-content text-fg text-[14.5px] leading-[1.68] tracking-[-0.003em] font-normal",
              isUser && "max-w-[580px] rounded-[14px_14px_4px_14px] bg-surface-2 px-3.5 py-2.5 text-left light:bg-surface-3",
            )}
          >
            {msg.blocks.map((part, i) => {
              if (part.kind === "text" && part.streaming && i !== lastIdx) {
                return renderPart({ ...part, streaming: false }, i, partCtx);
              }
              return renderPart(part, i, partCtx);
            })}
          </div>
          <Slot name="message.actions" />
        </div>
      </div>
    </MessageContext.Provider>
  );
}
