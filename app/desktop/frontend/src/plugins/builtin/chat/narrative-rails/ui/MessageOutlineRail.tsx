import { useMemo } from "react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { useActiveConversationMessages } from "@/plugins/builtin/agent/public/conversation";
import { messageOutline } from "@/plugins/builtin/chat/message/public/outline";
import { Pressable, ScrollArea } from "@/ui";
import { scrollToHeading, useTranscriptMap } from "../adapters/transcriptAnchors";

/**
 * The sections of the answer you are reading.
 *
 * Scoped to ONE assistant turn, not the whole thread: a turn is the unit that
 * gets long enough to lose your place in, while the thread already has the turn
 * rail on the other side. An outline over every turn would also have to
 * re-derive itself on every streamed token of the whole transcript, instead of
 * the one message that is growing.
 *
 * Renders into the gutter the reading column reserves for it; whether that gutter
 * exists at the current width is the column's decision, not this one's.
 */
export function MessageOutlineRail() {
  const t = useT();
  const messages = useActiveConversationMessages();
  const { visibleTurnId: visibleTurn } = useTranscriptMap();

  // The assistant turn being read: the visible one when it is an answer, else the
  // answer that follows the visible question — which is what you are heading into
  // when a user turn is under the reading line.
  const index = messages.findIndex((message) => message.id === visibleTurn);
  const target =
    index >= 0
      ? messages.slice(index).find((message) => message.role === "assistant")
      : messages.findLast((message) => message.role === "assistant");

  const entries = useMemo(() => (target ? messageOutline(target) : []), [target]);
  // One heading is not a contents list; it is the answer's own title, already on
  // screen two lines above where the rail would point.
  if (entries.length < 2) return null;

  const shallowest = Math.min(...entries.map((entry) => entry.level));

  return (
    <nav
      aria-label={t("narrative.rail.outline")}
      className="flex w-full flex-col gap-2 overflow-hidden pb-5 pl-9 pr-6 pt-9"
    >
      <span className="px-2 text-ui-2xs text-fg-faint">{t("narrative.rail.outline")}</span>
      <ScrollArea hideScrollbar className="min-h-0 flex-1">
        <div className="flex flex-col">
          {entries.map((entry) => (
            <Pressable
              key={`${entry.anchor}:${entry.index}`}
              type="button"
              data-chrome-focus=""
              title={entry.label}
              onClick={() => scrollToHeading(entry.anchor, entry.index)}
              className={cn(
                "flex min-w-0 items-start rounded-xs py-1 pr-1 text-left",
                "text-ui-xs leading-snug text-fg-muted transition-colors duration-[var(--dur-fast)]",
                "hover:text-fg",
              )}
              // Depth is the author's, so the rail shows it rather than flattening
              // six levels of structure into one list of equals.
              style={{ paddingLeft: `${8 + (entry.level - shallowest) * 10}px` }}
            >
              <span className="min-w-0 truncate">{entry.label}</span>
            </Pressable>
          ))}
        </div>
      </ScrollArea>
    </nav>
  );
}
