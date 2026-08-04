import type { CSSProperties } from "react";
import { useState } from "react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { formatClock } from "@/lib/i18n/relativeTime";
import { useActiveConversationMessages } from "@/plugins/builtin/agent/public/conversation";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { Pressable, RichTooltip } from "@/ui";
import { foldExchanges, scrollToTurn, useTranscriptMap } from "../adapters/transcriptAnchors";

/** How far the pointer's reach carries, in marks. Beyond this a mark is at its
 *  resting length. Three is enough to read as a swell and short enough that the
 *  run still shows the conversation's own shape underneath it. */
const REACH = 3;

/** Longest a mark gets from the pointer alone, in px. */
const MAGNIFY = 16;

/** Floor length, so a one-line exchange is a mark rather than a speck. */
const FLOOR = 8;

/** How much of a mark's length its share of the transcript can buy. */
const SHARE = 10;

/**
 * The track every mark is drawn in — the longest one of them can be.
 *
 * The rows are the tooltip's anchor, so this is also how far from the marks the
 * preview card opens. Stretched to the gutter (which is what `w-full` did) the
 * card detached from the rail entirely and opened a clear 260px away, over the
 * text, pointing at nothing.
 */
const TRACK = FLOOR + SHARE + MAGNIFY;

/**
 * A map of the conversation, one mark per question asked.
 *
 * One mark per exchange — a question and everything that answers it. A conversation's
 * shape is the questions in it; an assistant turn is the answer to the mark above it,
 * and giving both a mark would double the rail's length while halving what each one
 * tells you. The mark stays lit for the whole exchange, answer included, which is the
 * bug this once had: the highlight went out the moment you scrolled past the question
 * into its own answer.
 *
 * The marks are rules, not dots, and their resting length is the share of the
 * transcript that exchange occupies — measured, not guessed. Equal dots say only
 * "there were eight turns"; a scaled rule says which two of them are the whole
 * session, which is the question someone reaches for a map to answer.
 *
 * Renders into the gutter the reading column reserves for it; whether that gutter
 * exists at the current width is the column's decision, not this one's.
 */
export function TurnRail() {
  const t = useT();
  const messages = useActiveConversationMessages();
  const { visibleTurnId, turns: extents } = useTranscriptMap();
  const [reached, setReached] = useState<number | null>(null);
  // The same fold the measurement uses, so a mark exists for every exchange the
  // reading line can name. Derived from messages rather than from the measured
  // extents so the rail is drawn on first paint, before any layout has happened.
  const turns = foldExchanges(messages);
  if (turns.length < 2) return null;

  const shareOf = (id: string) => extents.find((extent) => extent.id === id)?.share ?? 0;
  // The pointer swells the marks around it, not just the one under it. At this
  // pitch a single mark changing length is a flicker you can miss; a swell tells
  // you where in the run you are reaching before you have read anything.
  const swell = (index: number) =>
    reached === null ? 0 : Math.max(0, 1 - Math.abs(index - reached) / REACH);

  return (
    // Centred as a block and packed tight. A map is read as a shape — the run of
    // marks together, and where in it you are — so it wants to be one small dense
    // object beside the text, not a ladder of widely spaced rungs starting at the
    // top of the pane.
    <nav
      aria-label={t("narrative.rail.turns")}
      className="flex h-full w-fit flex-col items-start justify-center overflow-hidden py-6 pl-6"
      onPointerLeave={() => setReached(null)}
    >
      {turns.map((turn, index) => {
        const active = turn.id === visibleTurnId;
        // The pointer outranks the reading position: while you are reaching into
        // the rail, the mark you are on is the one you want marked.
        const lead = reached === null ? active : reached === index;
        return (
          <RichTooltip
            key={turn.id}
            side="right"
            sideOffset={12}
            className="w-[276px] rounded-[var(--floating-panel-radius)] bg-card p-0"
            trigger={
              <Pressable
                type="button"
                data-chrome-focus=""
                aria-current={active ? "true" : undefined}
                aria-label={turnLabel(t, turn, index, turns.length)}
                onPointerEnter={() => setReached(index)}
                onFocus={() => setReached(index)}
                onBlur={() => setReached(null)}
                onClick={() => scrollToTurn(turn.id)}
                className="flex h-[9px] shrink-0 items-center"
                style={{ width: `${TRACK}px` } as CSSProperties}
              >
                {/* One thickness for every mark, the lead one's. A hairline that
                    doubles when it becomes current made the rail's resting state
                    read as a scratch and the change as a thickening rather than a
                    move — the length is the measurement here, and it is the only
                    thing that should be saying anything. */}
                <span
                  className={cn(
                    "h-[2px] rounded-pill transition-[background-color,width] duration-[var(--dur-fast)]",
                    // The one hand-picked alpha left in the tree, and deliberately:
                    // a resting mark is 2px of ink between the faint step and a
                    // hairline, and the ramp has no rung there — `fg-faint` reads as
                    // a scale of text and `border-soft` as an edge. `check-design-tokens`
                    // covers alpha on FILLS built from full ink, which this is not.
                    lead ? "bg-fg" : "bg-fg-faint/55",
                  )}
                  style={
                    {
                      width: `${FLOOR + Math.round(shareOf(turn.id) * SHARE + swell(index) * MAGNIFY)}px`,
                    } as CSSProperties
                  }
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
 * The question in one line and the answer's opening under it. No timestamp: when
 * you asked something is never what you are scanning a conversation for, and the
 * mark's own tooltip already carries it for assistive tech.
 */
function TurnPreview({ turn, answer }: { turn: Message; answer: Message | undefined }) {
  const t = useT();
  const question = proseOf(turn);
  const reply = proseOf(answer);

  return (
    <div className="flex flex-col gap-1.5 px-3.5 py-3 text-left">
      <span className="line-clamp-1 text-ui-md font-medium leading-snug text-fg">
        {question || t("role.user")}
      </span>
      {reply && <span className="line-clamp-3 text-ui-sm leading-body text-fg-muted">{reply}</span>}
    </div>
  );
}
