import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/classNames";
import { useScrollLock } from "./use-scroll-lock";

interface Props {
  /** Expanded when true; collapses the row to 0fr when false. */
  open: boolean;
  children: ReactNode;
}

/**
 * Vertical expand/collapse via `grid-template-rows: 0fr ↔ 1fr` — a NO-measurement
 * animation, and deliberately NOT Framer Motion `height: "auto"`.
 *
 * FM measures "auto" by briefly inflating the element to its natural height then
 * restoring it; the chat scroller's `use-stick-to-bottom` ResizeObserver reads that
 * transient as a content shrink and clamps the view to the top. A grid row's
 * grow/shrink is one REAL size change the sticky-bottom follows correctly. Reach for
 * THIS, not height:auto, for anything that expands inside the message stream.
 *
 * Children mount on first open and stay mounted so the close animates too, which is
 * why the collapsed row is `inert`: clipped content is still focusable and still read
 * aloud, so without it every collapsed tool card kept its buttons in the tab order
 * and its body in the accessibility tree.
 *
 * Collapsing a tall block sitting above the viewport would otherwise slide the outer
 * chat scroll as content vanishes; useScrollLock pins it for the animation window.
 */
export function Collapsible({ open, children }: Props) {
  const [revealed, setRevealed] = useState(open);
  const rowRef = useRef<HTMLDivElement>(null);
  const wasOpen = useRef(open);
  const lockScroll = useScrollLock(rowRef);

  useEffect(() => {
    if (wasOpen.current && !open) lockScroll();
    wasOpen.current = open;
  }, [open, lockScroll]);

  return (
    <div
      ref={rowRef}
      className={cn(
        // The column is stated because the grid is implicit and one column wide:
        // left to `auto` it sizes to the widest thing inside, so a long path in a
        // nested row pushed the whole card past the reading column and the overflow
        // was cut rather than truncated with an ellipsis.
        "grid grid-cols-[minmax(0,1fr)]",
        "transition-[grid-template-rows] duration-[var(--dur-disclosure)] ease-out",
        open ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}
      onTransitionRun={() => {
        if (open) setRevealed(true);
      }}
    >
      {/* `clip`, not `hidden`: both cut the same pixels, but `hidden` makes this box a
          scroll container, and a scroll container is the scrollport a `sticky`
          descendant measures against — which is how a tool group's sticky header,
          nested in a folded wave, came to have a port that never scrolls. */}
      <div inert={!open} className="min-h-0 overflow-clip">
        {(open || revealed) && children}
      </div>
    </div>
  );
}
