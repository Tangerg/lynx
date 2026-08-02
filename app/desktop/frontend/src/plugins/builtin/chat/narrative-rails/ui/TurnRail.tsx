import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { formatClock } from "@/lib/i18n/relativeTime";
import { useActiveConversationMessages } from "@/plugins/builtin/agent/public/conversation";
import { Pressable, Tooltip } from "@/ui";
import { scrollToTurn, useVisibleTurnId } from "../adapters/transcriptAnchors";

/**
 * One dot per question asked — the transcript's spatial map.
 *
 * Only user turns get a dot. A conversation's shape is the questions in it; an
 * assistant turn is the answer to the dot above it, and giving both a mark would
 * double the rail's length while halving what each mark tells you.
 *
 * Hidden until the reading column has room to spare: on a narrow window the
 * column is the scarce thing, and a navigation aid that takes width from the text
 * it navigates has stopped helping.
 */
export function TurnRail() {
  const t = useT();
  const messages = useActiveConversationMessages();
  const visibleTurn = useVisibleTurnId();
  const turns = messages.filter((message) => message.role === "user");
  if (turns.length < 2) return null;

  return (
    <nav
      aria-label={t("narrative.rail.turns")}
      className="hidden w-11 shrink-0 flex-col items-center gap-2 overflow-hidden pt-9 @min-[560px]:flex"
    >
      {turns.map((turn) => {
        const active = turn.id === visibleTurn;
        const stamp = formatClock(turn.createdAt);
        return (
          <Tooltip key={turn.id} label={stamp || t("role.user")} side="right">
            <Pressable
              type="button"
              data-chrome-focus=""
              aria-current={active ? "true" : undefined}
              aria-label={`${t("role.user")}${stamp ? ` · ${stamp}` : ""}`}
              onClick={() => scrollToTurn(turn.id)}
              className="grid size-[22px] shrink-0 place-items-center rounded-full transition-colors duration-[var(--dur-fast)] hover:bg-hover"
            >
              <span
                className={cn(
                  "size-2 rounded-full border transition-colors duration-[var(--dur-fast)]",
                  active ? "border-accent bg-accent" : "border-field-strong bg-transparent",
                )}
              />
            </Pressable>
          </Tooltip>
        );
      })}
    </nav>
  );
}
