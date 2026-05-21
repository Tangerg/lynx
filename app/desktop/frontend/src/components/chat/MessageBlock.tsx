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
            {msg.blocks.map((part, i) => renderPart(part, i, ctx))}
          </div>
          {/* Per-message action row (copy, retry, etc.). Empty by default. */}
          <Slot name="message.actions" />
        </div>
      </div>
    </MessageContext.Provider>
  );
}
