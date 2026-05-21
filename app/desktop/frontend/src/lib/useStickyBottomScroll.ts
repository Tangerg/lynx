import { useCallback, useEffect, useRef, useState, type RefObject } from "react";

// useStickyBottomScroll — keeps a scroll container glued to its content's
// bottom while the content grows, but yields control the moment the user
// scrolls away. Re-engages once the user scrolls all the way back to the
// literal scroll max.
//
// Semantics (in plain English):
//   1. DEFAULT: follow mode is ON — new content auto-scrolls into view.
//   2. User scrolls anywhere except the literal bottom → follow OFF.
//      They stay parked wherever they stopped.
//   3. User scrolls all the way back to the literal bottom → follow ON.
//
// How "is this scroll the user's or ours" is decided: we listen for
// device input events (wheel / touchmove / mousedown) that *precede* a
// user-initiated scroll, and arm a short timeout window. Scroll events
// that land inside the window are user-initiated; outside it they're
// ours (from a programmatic `scrollTop = scrollHeight`) and we ignore
// them. This avoids the trap where our own auto-scrolls would otherwise
// re-toggle follow mode on themselves.
//
// ResizeObserver on the inner content fires for every height change —
// backend deltas, smooth-text reveals, tool cards expanding — and
// performs the actual auto-scroll only when follow mode is on.
//
// `resetKey` forces follow mode back ON and snaps to the bottom when it
// changes (e.g. switching sessions / threads).
//
// Returns `{ atBottom, scrollToBottom }` so the caller can render a
// "jump to bottom" affordance and programmatically re-engage follow
// when the user clicks it. `atBottom` mirrors the ref but is React
// state so it re-renders consumers; the ref stays the fast path used
// inside the rAF/ResizeObserver loop.
export type StickyBottomControls = {
  atBottom: boolean;
  scrollToBottom: () => void;
};

export function useStickyBottomScroll<T>(
  scrollRef: RefObject<HTMLDivElement | null>,
  resetKey: T,
): StickyBottomControls {
  const followRef = useRef(true);
  const [atBottom, setAtBottom] = useState(true);

  // The single place follow-state mutates. Updates the ref (read by the
  // RO callback) AND the mirrored React state (read by consumers /
  // JumpToBottomButton). setState short-circuits on equal values so
  // it's safe to call indiscriminately on every scroll tick.
  const setFollow = useCallback((next: boolean) => {
    followRef.current = next;
    setAtBottom(next);
  }, []);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    let userInputTimeout: number | null = null;
    const USER_INPUT_WINDOW_MS = 200;

    const markUserInput = () => {
      if (userInputTimeout !== null) clearTimeout(userInputTimeout);
      userInputTimeout = window.setTimeout(() => {
        userInputTimeout = null;
      }, USER_INPUT_WINDOW_MS);
    };

    el.addEventListener("wheel", markUserInput, { passive: true });
    el.addEventListener("touchmove", markUserInput, { passive: true });
    el.addEventListener("mousedown", markUserInput);

    const onScroll = () => {
      if (userInputTimeout === null) return; // programmatic, ignore
      const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
      setFollow(dist <= 1);
    };
    el.addEventListener("scroll", onScroll, { passive: true });

    const ro = new ResizeObserver(() => {
      if (followRef.current) {
        el.scrollTop = el.scrollHeight;
      }
    });
    const inner = el.firstElementChild;
    if (inner) ro.observe(inner);

    return () => {
      el.removeEventListener("wheel", markUserInput);
      el.removeEventListener("touchmove", markUserInput);
      el.removeEventListener("mousedown", markUserInput);
      el.removeEventListener("scroll", onScroll);
      ro.disconnect();
      if (userInputTimeout !== null) clearTimeout(userInputTimeout);
    };
    // The effect only sets up listeners once; refs hold the live state.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // scrollToBottom is the programmatic "back to live tail" — snaps the
  // scroll position AND re-arms follow. The programmatic scroll itself
  // doesn't disengage follow because the input-timeout gate in
  // onScroll filters it out.
  const scrollToBottom = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    setFollow(true);
  }, [scrollRef, setFollow]);

  // Reset on key change — equivalent to a user-triggered scrollToBottom.
  useEffect(() => {
    scrollToBottom();
  }, [resetKey, scrollToBottom]);

  return { atBottom, scrollToBottom };
}
