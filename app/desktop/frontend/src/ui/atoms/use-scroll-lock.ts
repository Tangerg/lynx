import type { RefObject } from "react";
import { useCallback, useEffect, useRef } from "react";

/**
 * Pins the nearest scrollable ancestor's scroll position for the duration of a
 * height animation, so a growing/shrinking block doesn't yank the surrounding
 * view. Returns a function to call right before the height change begins.
 *
 * Why this exists: when a block's height changes mid-scroll the browser's own
 * scroll anchoring / clamping can shift the viewport, and the sticky-bottom
 * chat scroller reads the transient as a jump. We snapshot scrollTop, hide the
 * scrollbar (compensating its gutter so centered content doesn't slide), and
 * re-assert the snapshot on every scroll event until the animation window
 * closes — then restore everything.
 *
 * @param animatedElementRef ref to the element whose height animates
 */
export function useScrollLock<T extends HTMLElement = HTMLElement>(
  animatedElementRef: RefObject<T | null>,
) {
  const scrollContainerRef = useRef<HTMLElement | null>(null);
  const cleanupRef = useRef<(() => void) | null>(null);

  // A lock in flight when the component unmounts would leave the scrollbar
  // hidden and the padding shim in place; tear it down.
  useEffect(() => () => cleanupRef.current?.(), []);

  return useCallback(() => {
    cleanupRef.current?.();

    if (!scrollContainerRef.current && animatedElementRef.current) {
      let el: HTMLElement | null = animatedElementRef.current;
      while (el) {
        const { overflowY } = getComputedStyle(el);
        if (overflowY === "scroll" || overflowY === "auto") {
          scrollContainerRef.current = el;
          break;
        }
        el = el.parentElement;
      }
    }

    const scrollContainer = scrollContainerRef.current;
    const animatedElement = animatedElementRef.current;
    if (!scrollContainer || !animatedElement) return;

    const scrollPosition = scrollContainer.scrollTop;
    const previousScrollbarWidth = scrollContainer.style.scrollbarWidth;

    // Hiding the scrollbar collapses its gutter on classic scrollbars, which
    // shifts centered content horizontally; compensate with padding on the
    // side the scrollbar occupies (the left side in RTL).
    const computed = getComputedStyle(scrollContainer);
    const paddingSide = computed.direction === "rtl" ? "paddingLeft" : "paddingRight";
    const previousPadding = scrollContainer.style[paddingSide];
    const scrollbarSize =
      scrollContainer.offsetWidth -
      scrollContainer.clientWidth -
      parseFloat(computed.borderLeftWidth) -
      parseFloat(computed.borderRightWidth);

    scrollContainer.style.scrollbarWidth = "none";
    if (scrollbarSize > 0) {
      scrollContainer.style[paddingSide] = `${parseFloat(computed[paddingSide]) + scrollbarSize}px`;
    }

    const restoreStyles = () => {
      scrollContainer.style.scrollbarWidth = previousScrollbarWidth;
      scrollContainer.style[paddingSide] = previousPadding;
    };

    const resetPosition = () => {
      scrollContainer.scrollTop = scrollPosition;
    };
    scrollContainer.addEventListener("scroll", resetPosition);

    const timeoutId = setTimeout(
      () => {
        scrollContainer.removeEventListener("scroll", resetPosition);
        restoreStyles();
        cleanupRef.current = null;
      },
      transitionWindowMs(getComputedStyle(animatedElement)),
    );

    cleanupRef.current = () => {
      clearTimeout(timeoutId);
      scrollContainer.removeEventListener("scroll", resetPosition);
      restoreStyles();
    };
  }, [animatedElementRef]);
}

function transitionWindowMs(style: CSSStyleDeclaration): number {
  const durations = style.transitionDuration.split(",").map(cssTimeMs);
  const delays = style.transitionDelay.split(",").map(cssTimeMs);
  return durations.reduce(
    (longest, duration, index) =>
      Math.max(longest, duration + (delays[index % delays.length] ?? 0)),
    0,
  );
}

function cssTimeMs(value: string): number {
  const time = Number.parseFloat(value);
  if (!Number.isFinite(time)) return 0;
  return value.trim().endsWith("ms") ? time : time * 1_000;
}
