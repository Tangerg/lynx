import type { ReactNode, RefObject } from "react";
import { useLayoutEffect, useRef } from "react";
import { cn } from "@/lib/classNames";
import { JumpToBottomButton } from "./JumpToBottomButton";
import { COMPOSER_OVERLAY_PROPERTY, READING_COLUMN, READING_GUTTER } from "./readingColumn";

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
  publishHeightTo,
  children,
}: {
  /**
   * Where to write `--composer-overlay`. The transcript pads its tail by that
   * height so its last message can come out from under this panel, and it is a
   * SIBLING of this one — so the number has to land on an ancestor they share.
   */
  publishHeightTo: RefObject<HTMLElement | null>;
  children: ReactNode;
}) {
  const overlay = useRef<HTMLDivElement>(null);

  // Written straight to the element rather than held in state: the composer
  // resizes on the keystroke that wraps a line, and routing that through a
  // render would put the whole message list on the typing path.
  //
  // The target is resolved inside the callback, and it has to be. React attaches
  // refs child-first, so when this effect runs the ANCESTOR named by
  // `publishHeightTo` has no element yet — and an effect that reads it here and
  // bails on null never runs again, because a ref object is stable and nothing
  // ever invalidates the dep. The observer fires after the commit, by which time
  // the ancestor is mounted, so reading it there is both correct and the only
  // place it can be read. Hoisting it out for "one less deref" silently gives the
  // transcript zero clearance and hands the composer the tail of every message.
  useLayoutEffect(() => {
    const element = overlay.current;
    if (!element) return;
    const observer = new ResizeObserver(() => {
      publishHeightTo.current?.style.setProperty(
        COMPOSER_OVERLAY_PROPERTY,
        `${element.offsetHeight}px`,
      );
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, [publishHeightTo]);

  return (
    <div
      ref={overlay}
      className={cn("pointer-events-none absolute inset-x-0 bottom-0 z-10", READING_COLUMN)}
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
