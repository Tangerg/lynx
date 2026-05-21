// MessageBlock — renders one chat turn.
//
// Role identity (display name + avatar icon) is looked up from the
// plugin registry. Fallbacks render a generic identity when no plugin
// has registered the role.

import { Icon, type IconName } from "@/components/common";
import { Avatar } from "@/components/common/Avatar";
import { Slot } from "@/plugins/Slot";
import { useMessageRole } from "@/plugins/sdk";
import type { Message } from "@/protocol/agui/viewState";
import { MessageContext } from "./MessageContext";
import { renderPart, type PartCtx } from "./PartRenderer";

export function MessageBlock({ msg, ctx }: { msg: Message; ctx: PartCtx }) {
  const role = useMessageRole(msg.role);
  const isUser = msg.role === "user";

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
  // Force user blocks to render synchronously.
  const partCtx: PartCtx = isUser ? { ...ctx, instant: true } : ctx;

  return (
    <MessageContext.Provider value={msg}>
      <div className={`msg ${msg.role === "assistant" ? "agent" : msg.role}`}>
        <Avatar variant={avatarVariant}>
          <Icon name={iconName} size={14} />
        </Avatar>
        <div className="msg-body">
          <div className="msg-meta">
            <span className={`who ${isUser ? "user" : "agent"}`}>{displayName}</span>
            <span>·</span>
            <span>{msg.time}</span>
            {/* Right-aligned slot for badges, status pips, copy buttons. */}
            <Slot name="message.header.end" />
          </div>
          <div className="msg-content">
            {msg.blocks.map((part, i) => {
              // Strip `streaming: true` from any text block that isn't the
              // last one in the message, so only the active tail shows the
              // cursor.
              if (part.kind === "text" && part.streaming && i !== lastIdx) {
                return renderPart({ ...part, streaming: false }, i, partCtx);
              }
              return renderPart(part, i, partCtx);
            })}
          </div>
          {/* Per-message action row (copy, retry, etc.). Empty by default. */}
          <Slot name="message.actions" />
        </div>
      </div>
    </MessageContext.Provider>
  );
}
