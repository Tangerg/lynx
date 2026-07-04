import type { ReactNode } from "react";
import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import { useScrollLock } from "./use-scroll-lock";

interface Props {
  /** Expanded when true; collapses the row to 0fr when false. */
  open: boolean;
  children: ReactNode;
}

// Must match the `duration-150` transition below — useScrollLock holds the
// scroll position for exactly the animation window, no longer.
const ANIMATION_MS = 150;

/**
 * Vertical expand/collapse via `grid-template-rows: 0fr ↔ 1fr` — a
 * NO-measurement animation, and deliberately NOT Framer Motion `height: "auto"`.
 *
 * FM measures "auto" by briefly inflating the element to its natural height
 * then restoring it; the chat scroller's `use-stick-to-bottom` ResizeObserver
 * reads that transient as a content shrink and clamps the view to the top (the
 * "D1" scroll jump). A grid row's grow/shrink is instead a single REAL size
 * change the sticky-bottom follows correctly. Reach for THIS, not height:auto,
 * for anything that expands inside the message stream.
 *
 * Children mount on first open and stay mounted (hidden by the collapsed row)
 * so the close animates too; `min-h-0` lets the row shrink below content height.
 *
 * Collapsing a tall block sitting above the viewport would otherwise slide the
 * outer chat scroll as content vanishes; useScrollLock pins it for the
 * animation window.
 */
export function Collapsible({ open, children }: Props) {
  const [revealed, setRevealed] = useState(open);
  const rowRef = useRef<HTMLDivElement>(null);
  const wasOpen = useRef(open);
  const lockScroll = useScrollLock(rowRef, ANIMATION_MS);

  useEffect(() => {
    if (open) setRevealed(true);
  }, [open]);

  useEffect(() => {
    if (wasOpen.current && !open) lockScroll();
    wasOpen.current = open;
  }, [open, lockScroll]);

  return (
    <div
      ref={rowRef}
      className={cn(
        "grid transition-[grid-template-rows] duration-150 ease-out",
        open ? "grid-rows-[1fr]" : "grid-rows-[0fr]",
      )}
    >
      <div className="min-h-0 overflow-hidden">{(open || revealed) && children}</div>
    </div>
  );
}
