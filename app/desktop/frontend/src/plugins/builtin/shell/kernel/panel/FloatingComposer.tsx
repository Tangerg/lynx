import type { ReactNode, RefObject } from "react";
import { cn } from "@/lib/classNames";
import { JumpToBottomButton } from "./JumpToBottomButton";
import { READING_COLUMN, READING_GUTTER } from "./readingColumn";

/**
 * The composer, resting over the tail of the transcript.
 *
 * Paints NOTHING of its own — the panel it holds is glass, and a backing behind
 * glass is just an opaque bar with a translucent sticker on it. What keeps the text
 * from colliding with the panel is the scroller's own dissolve
 * (`.msg-scroll-viewport`), which fades the last strip out; the text under the
 * panel itself stays, blurred, because that is the whole point of the material.
 *
 * Exactly the COLUMN wide, never the pane. A full-width overlay is a bottom bar
 * however it is positioned: it paints across the whole pane, which reads as
 * chrome, and it takes the scrollbar's bottom inch with it. Nothing outside the
 * column has anything to hide anyway — the transcript is centred and capped.
 */
export function FloatingComposer({
  overlayRef,
  children,
}: {
  /** Shared with ChatStream, the layout owner that reserves this overlay's height. */
  overlayRef: RefObject<HTMLDivElement | null>;
  children: ReactNode;
}) {
  return (
    <div
      ref={overlayRef}
      className={cn("pointer-events-none absolute inset-x-0 bottom-0 z-2", READING_COLUMN)}
    >
      <div className={cn(READING_GUTTER, "pb-3 sm:pb-4")}>
        <div className="pointer-events-auto relative">
          <JumpToBottomButton />
          {children}
        </div>
      </div>
    </div>
  );
}
