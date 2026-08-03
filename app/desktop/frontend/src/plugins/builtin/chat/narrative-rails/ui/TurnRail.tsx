import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { formatClock } from "@/lib/i18n/relativeTime";
import { useActiveConversationMessages } from "@/plugins/builtin/agent/public/conversation";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { Pressable, RichTooltip } from "@/ui";
import { scrollToTurn, useTranscriptMap } from "../adapters/transcriptAnchors";

/**
 * A map of the conversation, one mark per question asked.
 *
 * Only user turns get a mark. A conversation's shape is the questions in it; an
 * assistant turn is the answer to the mark above it, and giving both a mark
 * would double the rail's length while halving what each one tells you.
 *
 * The marks are rules, not dots, and their length is the share of the transcript
 * that exchange occupies — measured, not guessed. Equal dots say only "there were
 * eight turns"; a scaled rule says which two of them are the whole session, which
 * is the question someone reaches for a map to answer.
 *
 * Renders into the gutter the reading column reserves for it; whether that gutter
 * exists at the current width is the column's decision, not this one's.
 */
export function TurnRail() {
  const t = useT();
  const messages = useActiveConversationMessages();
  const { visibleTurnId, turns: extents } = useTranscriptMap();
  const turns = messages.filter((message) => message.role === "user");
  if (turns.length < 2) return null;

  const shareOf = (id: string) => extents.find((extent) => extent.id === id)?.share ?? 0;

  return (
    <nav
      aria-label={t("narrative.rail.turns")}
      className="flex w-full flex-col items-stretch gap-2.5 overflow-hidden py-9 pl-5 pr-3"
    >
      {turns.map((turn, index) => {
        const active = turn.id === visibleTurnId;
        return (
          <RichTooltip
            key={turn.id}
            side="right"
            sideOffset={10}
            className="w-[300px] p-0"
            trigger={
              <Pressable
                type="button"
                data-chrome-focus=""
                aria-current={active ? "true" : undefined}
                aria-label={turnLabel(t, turn, index, turns.length)}
                onClick={() => scrollToTurn(turn.id)}
                className="group/turn flex h-3 shrink-0 items-center"
              >
                <span
                  className={cn(
                    "h-0.5 rounded-pill transition-[background-color,height] duration-[var(--dur-fast)]",
                    active ? "h-[3px] bg-fg" : "bg-fg-faint/45 group-hover/turn:bg-fg-muted",
                  )}
                  // A floor so a one-line exchange is still a mark rather than a
                  // speck, and a ceiling so the longest never touches the column.
                  style={{ width: `${28 + Math.round(shareOf(turn.id) * 72)}%` }}
                />
              </Pressable>
            }
          >
            <TurnPreview turn={turn} answer={answerAfter(messages, turn.id)} />
          </RichTooltip>
        );
      })}
    </nav>
  );
}

function turnLabel(
  t: ReturnType<typeof useT>,
  turn: Message,
  index: number,
  total: number,
): string {
  const stamp = formatClock(turn.createdAt);
  return `${t("role.user")} ${index + 1}/${total}${stamp ? ` · ${stamp}` : ""}`;
}

function answerAfter(messages: Message[], turnId: string): Message | undefined {
  const index = messages.findIndex((message) => message.id === turnId);
  if (index < 0) return undefined;
  return messages.slice(index + 1).find((message) => message.role === "assistant");
}

/** First prose of a message, flattened to one paragraph's worth of plain text. */
function proseOf(message: Message | undefined): string {
  if (!message) return "";
  for (const block of message.blocks) {
    if (block.kind !== "text") continue;
    const plain = block.text
      .replace(/```[\s\S]*?```/g, " ")
      .replace(/[#>*_`~-]/g, " ")
      .replace(/\s+/g, " ")
      .trim();
    if (plain) return plain;
  }
  return "";
}

/**
 * What that exchange was about, without going back to it.
 *
 * The question in full voice and the answer's opening underneath — a timestamp
 * alone told you when you asked something, which is never the thing you are
 * scanning a conversation for.
 */
function TurnPreview({ turn, answer }: { turn: Message; answer: Message | undefined }) {
  const t = useT();
  const question = proseOf(turn);
  const reply = proseOf(answer);
  const stamp = formatClock(turn.createdAt);

  return (
    <div className="flex flex-col gap-1.5 px-3 py-2.5 text-left">
      <span className="line-clamp-2 text-ui-sm font-medium leading-snug text-fg">
        {question || t("role.user")}
      </span>
      {reply && <span className="line-clamp-3 text-ui-xs leading-body text-fg-muted">{reply}</span>}
      {stamp && <span className="font-mono text-ui-2xs text-fg-faint">{stamp}</span>}
    </div>
  );
}
