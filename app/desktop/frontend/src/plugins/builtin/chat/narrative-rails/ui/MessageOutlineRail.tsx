import { useMemo } from "react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { useActiveConversationMessages } from "@/plugins/builtin/agent/public/conversation";
import { useActiveSessionToolCalls } from "@/plugins/builtin/agent/public/run";
import { messageOutline } from "@/plugins/builtin/chat/message/public/outline";
import { Pressable, ScrollArea } from "@/ui";
import { scrollToBlock, useVisibleTurnId } from "../adapters/transcriptAnchors";

/**
 * The sections of the answer you are reading.
 *
 * Scoped to ONE assistant turn, not the whole thread: a turn is the unit that
 * gets long enough to lose your place in — twenty tool calls, a plan, a diff and
 * a table — while the thread already has the turn rail on the other side. An
 * outline over every turn would also have to re-derive itself on every streamed
 * token of the whole transcript, instead of the one message that is growing.
 */
export function MessageOutlineRail() {
  const t = useT();
  const messages = useActiveConversationMessages();
  const toolCalls = useActiveSessionToolCalls();
  const visibleTurn = useVisibleTurnId();

  // The assistant turn being read: the visible one when it is an answer, else the
  // answer that follows the visible question — which is what you are heading into
  // when a user turn is under the reading line.
  const index = messages.findIndex((message) => message.id === visibleTurn);
  const target =
    index >= 0
      ? messages.slice(index).find((message) => message.role === "assistant")
      : messages.findLast((message) => message.role === "assistant");

  const entries = useMemo(
    () => (target ? messageOutline(t, target, toolCalls) : []),
    [t, target, toolCalls],
  );

  if (entries.length < 3) return null;

  return (
    <nav
      aria-label={t("narrative.rail.outline")}
      className="hidden w-[186px] shrink-0 flex-col gap-2 pb-5 pl-1 pr-4 pt-9 @min-[900px]:flex"
    >
      <span className="px-2 text-ui-2xs text-fg-faint">{t("narrative.rail.outline")}</span>
      <ScrollArea hideScrollbar className="min-h-0 flex-1">
        <div className="flex flex-col">
          {entries.map((entry) => (
            <Pressable
              key={entry.anchor}
              type="button"
              data-chrome-focus=""
              title={entry.label}
              onClick={() => scrollToBlock(entry.anchor)}
              className={cn(
                "flex min-w-0 items-start gap-2 rounded-xs py-1 pl-2 pr-1 text-left",
                "text-ui-xs leading-snug text-fg-muted transition-colors duration-[var(--dur-fast)]",
                "hover:text-fg",
              )}
            >
              <span className="min-w-0 truncate">{entry.label}</span>
            </Pressable>
          ))}
        </div>
      </ScrollArea>
    </nav>
  );
}
