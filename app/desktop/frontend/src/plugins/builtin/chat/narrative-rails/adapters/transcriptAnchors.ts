import { useEffect, useState } from "react";

/** Written by the transcript on every rendered turn; read here to place the
 *  rails against what is actually on screen. */
export const TURN_ANCHOR_ATTR = "data-turn-id";
/** Also written by the transcript. What makes an exchange's boundary findable
 *  from the DOM without the rail having to know the conversation model. */
const TURN_ROLE_ATTR = "data-turn-role";
const TURN_SELECTOR = `[${TURN_ANCHOR_ATTR}]`;

/** Where the reading line sits, as a fraction of the scroller's height. A turn
 *  counts as "the one you are reading" once its top has passed this line. */
const READING_LINE = 0.35;

/** The transcript's scroll box. Named by the transcript itself (see
 *  MessageStream) rather than derived from the scroll library's DOM shape. */
function scroller(): HTMLElement | null {
  return document.querySelector<HTMLElement>(".msg-scroll-viewport");
}

function turnElements(root: HTMLElement): HTMLElement[] {
  return [...root.querySelectorAll<HTMLElement>(TURN_SELECTOR)];
}

/** How much of the transcript one EXCHANGE occupies, as a fraction of the longest
 *  one. This is a measurement, not an estimate: the rail draws the document it is a
 *  map of. */
export interface TurnExtent {
  id: string;
  share: number;
}

/** One anchored turn as the DOM reports it, before exchanges are folded out. */
export interface AnchoredTurn {
  id: string;
  role: string | null;
  top: number;
}

/**
 * Fold anchored messages into exchanges — a question and everything that answers it.
 *
 * The rail's unit has always been the exchange (one mark per question, the answer
 * belongs to the mark above it) while this module's unit was the message, and that
 * mismatch was three bugs at once: the highlight went out as soon as you scrolled
 * from a question into its answer, because the id under the reading line was the
 * assistant's and no mark carried it; every mark rested at the floor length, because
 * a question's "share of the transcript" was measured as the height of its own
 * bubble; and a compaction note between two turns counted as a turn of its own.
 *
 * A user anchor opens an exchange and everything after it belongs to that one. A
 * transcript that opens with something else (a restored assistant turn, a compaction
 * note) still gets a first exchange, so nothing is left unattributed.
 *
 * Generic over `{id, role}` so the rail can fold its MESSAGES with the same function
 * that folds these anchors. Two callers applying one rule cannot disagree about where
 * an exchange begins; two callers each filtering for `role === "user"` could, and did
 * — at the head of a restored transcript the measurement named an exchange the rail
 * had no mark for.
 */
export function foldExchanges<T extends { id: string; role: string | null }>(
  turns: readonly T[],
): T[] {
  const exchanges: T[] = [];
  for (const turn of turns) {
    if (turn.role === "user" || exchanges.length === 0) exchanges.push(turn);
  }
  return exchanges;
}

export interface TranscriptMap {
  /** The exchange currently under the reading line, named by its question. */
  visibleTurnId: string | null;
  /** Every exchange, in document order, with its share of the tallest. */
  turns: TurnExtent[];
}

const EMPTY: TranscriptMap = { visibleTurnId: null, turns: [] };

function sameMap(a: TranscriptMap, b: TranscriptMap): boolean {
  if (a.visibleTurnId !== b.visibleTurnId || a.turns.length !== b.turns.length) return false;
  return a.turns.every((turn, i) => {
    const other = b.turns[i]!;
    // Quantised: a streaming answer grows by a pixel a frame, and a rail that
    // re-rendered on every pixel would repaint sixty times a second to move a
    // tick by nothing a reader can see.
    return turn.id === other.id && Math.round(turn.share * 20) === Math.round(other.share * 20);
  });
}

/**
 * Where the reader is, and what the transcript looks like from above.
 *
 * Measured from the DOM on scroll rather than tracked in the store, because both
 * questions are geometric: which turn's box straddles the reading line, and how
 * tall each turn's box is. The transcript is a single scroller whose children
 * are re-rendered on every streamed token, so a React-side answer would either
 * re-render the whole list to compute it or go stale the moment a block above
 * grew.
 *
 * One hook for both, because both come from the same pass over the same boxes —
 * two hooks would install two scroll listeners and force layout twice a frame.
 *
 * `requestAnimationFrame` coalescing keeps this to one measurement per frame; a
 * bare scroll handler measured 60+ times a second and forced a layout each time.
 */
export function useTranscriptMap(): TranscriptMap {
  const [map, setMap] = useState<TranscriptMap>(EMPTY);

  useEffect(() => {
    const root = scroller();
    if (!root) return;

    let frame = 0;
    const measure = () => {
      frame = 0;
      const rootTop = root.getBoundingClientRect().top;
      const line = rootTop + root.clientHeight * READING_LINE;
      // The content's own bottom, in the same viewport coordinates as every
      // `getBoundingClientRect().top` below. The previous expression subtracted a
      // viewport offset from `scrollHeight + scrollTop`, which is content space —
      // so the last exchange measured thousands of pixels tall, it won `tallest`,
      // and every other mark's share rounded to nothing. That is why a rail whose
      // whole point is proportional marks drew ten identical stubs.
      const contentBottom = rootTop + root.scrollHeight - root.scrollTop;
      const anchored = turnElements(root).map((element) => ({
        id: element.getAttribute(TURN_ANCHOR_ATTR) ?? "",
        role: element.getAttribute(TURN_ROLE_ATTR),
        top: element.getBoundingClientRect().top,
      }));
      const exchanges = foldExchanges(anchored);
      const measured = exchanges.map((exchange, i) => ({
        id: exchange.id,
        top: exchange.top,
        // An exchange runs until the next question starts; the last runs to the
        // bottom of the content.
        height: (exchanges[i + 1]?.top ?? contentBottom) - exchange.top,
      }));
      const tallest = Math.max(1, ...measured.map((turn) => turn.height));

      let current: string | null = null;
      for (const turn of measured) {
        if (turn.top > line) break;
        current = turn.id;
      }

      const next: TranscriptMap = {
        // The first exchange owns the space above the reading line, so a transcript
        // scrolled to the very top still highlights something.
        visibleTurnId: current ?? measured[0]?.id ?? null,
        turns: measured.map((turn) => ({ id: turn.id, share: turn.height / tallest })),
      };
      setMap((previous) => (sameMap(previous, next) ? previous : next));
    };
    const schedule = () => {
      if (frame === 0) frame = requestAnimationFrame(measure);
    };

    measure();
    root.addEventListener("scroll", schedule, { passive: true });
    // The set of turns and their heights both change while a run streams, and
    // neither fires a scroll event.
    const observer = new ResizeObserver(schedule);
    observer.observe(root);
    const mutations = new MutationObserver(schedule);
    mutations.observe(root, { childList: true, subtree: true });

    return () => {
      if (frame !== 0) cancelAnimationFrame(frame);
      root.removeEventListener("scroll", schedule);
      observer.disconnect();
      mutations.disconnect();
    };
  }, []);

  return map;
}

/** Bring an element inside the transcript to the top of the reading area. */
function scrollIntoTranscript(target: HTMLElement | null): void {
  const root = scroller();
  if (!root || !target) return;
  const offset = target.getBoundingClientRect().top - root.getBoundingClientRect().top;
  root.scrollTo({ top: root.scrollTop + offset - 24, behavior: "smooth" });
}

export function scrollToTurn(id: string): void {
  scrollToAnchored(TURN_ANCHOR_ATTR, id);
}

function scrollToAnchored(attribute: string, value: string): void {
  const root = scroller();
  scrollIntoTranscript(
    root?.querySelector<HTMLElement>(`[${attribute}="${CSS.escape(value)}"]`) ?? null,
  );
}
