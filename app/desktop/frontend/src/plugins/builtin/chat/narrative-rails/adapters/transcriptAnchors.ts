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

/**
 * The turn the reader is currently in.
 *
 * Measured from the DOM on scroll rather than tracked in the store, because the
 * question is geometric: which turn's box currently straddles the reading line.
 * The transcript is a single scroller whose children are re-rendered on every
 * streamed token, so a React-side answer would either re-render the whole list
 * to compute it or go stale the moment a block above grew.
 *
 * `requestAnimationFrame` coalescing keeps this to one measurement per frame; a
 * bare scroll handler measured 60+ times a second and forced a layout each time.
 */
export function useVisibleTurnId(): string | null {
  const [visible, setVisible] = useState<string | null>(null);

  useEffect(() => {
    const root = scroller();
    if (!root) return;

    let frame = 0;
    const measure = () => {
      frame = 0;
      const line = root.getBoundingClientRect().top + root.clientHeight * READING_LINE;
      let current: string | null = null;
      for (const element of turnElements(root)) {
        if (element.getBoundingClientRect().top > line) break;
        current = element.getAttribute(TURN_ANCHOR_ATTR);
      }
      // The first turn owns the space above the reading line, so a transcript
      // scrolled to the very top still highlights something.
      setVisible(current ?? turnElements(root)[0]?.getAttribute(TURN_ANCHOR_ATTR) ?? null);
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

  return visible;
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

/** The message context publishes the attribute its blocks carry; finding one in
 *  the document is this adapter's job, because "where is it" is a question only
 *  the browser can answer. */
export function scrollToBlock(anchor: string): void {
  scrollToAnchored(BLOCK_ANCHOR_ATTR, anchor);
}

function scrollToAnchored(attribute: string, value: string): void {
  const root = scroller();
  scrollIntoTranscript(
    root?.querySelector<HTMLElement>(`[${attribute}="${CSS.escape(value)}"]`) ?? null,
  );
}
