import { useEffect, useRef, type RefObject } from "react";

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
// changes (e.g. switching sessions / threads). Pass the active id you
// want to reset on.
export function useStickyBottomScroll<T>(
  scrollRef: RefObject<HTMLDivElement | null>,
  resetKey: T,
) {
  const followRef = useRef(true);

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
      followRef.current = dist <= 1;
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

  // Reset on key change — snap to bottom + re-arm follow. The
  // programmatic scroll won't disengage follow because the input-timeout
  // gate in onScroll filters it out.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    followRef.current = true;
  }, [resetKey, scrollRef]);
}
