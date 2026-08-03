import { useEffect, useState } from "react";
import { BLOCK_ANCHOR_ATTR } from "@/plugins/builtin/chat/message/public/outline";

/** Written by the transcript on every rendered turn; read here to place the
 *  rails against what is actually on screen. */
export const TURN_ANCHOR_ATTR = "data-turn-id";
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

/** How much of the transcript one turn occupies, as a fraction of its longest
 *  sibling. This is a measurement, not an estimate: the rail draws the document
 *  it is a map of. */
export interface TurnExtent {
  id: string;
  share: number;
}

export interface TranscriptMap {
  /** The turn currently under the reading line. */
  visibleTurnId: string | null;
  /** Every turn, in document order, with its share of the tallest. */
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
      const line = root.getBoundingClientRect().top + root.clientHeight * READING_LINE;
      const elements = turnElements(root);
      const measured = elements.map((element, i) => ({
        id: element.getAttribute(TURN_ANCHOR_ATTR) ?? "",
        top: element.getBoundingClientRect().top,
        // A turn runs until the next one starts; the last runs to the bottom of
        // the content. The element itself is only the question — its answer is a
        // sibling, not a child.
        height:
          (elements[i + 1]?.getBoundingClientRect().top ?? root.scrollHeight + root.scrollTop) -
          element.getBoundingClientRect().top,
      }));
      const tallest = Math.max(1, ...measured.map((turn) => turn.height));

      let current: string | null = null;
      for (const turn of measured) {
        if (turn.top > line) break;
        current = turn.id;
      }

      const next: TranscriptMap = {
        // The first turn owns the space above the reading line, so a transcript
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

/**
 * Bring one heading inside a prose block into view.
 *
 * The message context publishes the attribute its blocks carry; finding one in
 * the document is this adapter's job, because "where is it" is a question only
 * the browser can answer.
 *
 * Addressed by position rather than by an id minted onto the element: the
 * outline and the rendered markdown are two readings of the same document, so
 * their headings are already in the same order, and an id would be a second
 * spelling of that fact for both sides to drift apart on.
 */
export function scrollToHeading(anchor: string, index: number): void {
  const root = scroller();
  const block = root?.querySelector<HTMLElement>(`[${BLOCK_ANCHOR_ATTR}="${CSS.escape(anchor)}"]`);
  const heading = block?.querySelectorAll<HTMLElement>("h1, h2, h3, h4, h5, h6")[index];
  scrollIntoTranscript(heading ?? block ?? null);
}

function scrollToAnchored(attribute: string, value: string): void {
  const root = scroller();
  scrollIntoTranscript(
    root?.querySelector<HTMLElement>(`[${attribute}="${CSS.escape(value)}"]`) ?? null,
  );
}
