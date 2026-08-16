import type { ComponentProps } from "react";
import { cn } from "@/lib/classNames";

// The shell every floating panel wears.
//
// Frosted rather than opaque: at a popover's size, letting a hint of what it covers
// show through reads as glass above the surface where a solid fill reads as a second
// page pasted over it. The blur sits on a `before` layer so it composites UNDER the
// content instead of blurring it. A surface big enough to cover a whole column is a
// modal instead and takes an opaque fill — see SearchOverlay.
//
// No border: the edge is the first layer of `--shadow-popover`, so a floating edge
// follows the same contrast algorithm as every region boundary. A border here would
// be the second line on one boundary.

const FLOATING_SURFACE_BASE = [
  // `isolate`, not a layer: the only thing this needs a stacking context for is
  // to keep the blur's `-z-1` above its own background. A z-index here reads as
  // competing with the page and cannot — the positioner's transform boxes it in.
  "relative isolate overflow-hidden text-fg",
  "bg-[var(--app-floating-surface)] shadow-[var(--shadow-popover)]",
  "before:pointer-events-none before:absolute before:inset-0 before:-z-1",
  // The blur reads from the style, like the fill above it — the two are one
  // recipe, and spelling the fill as a token while the blur stayed a utility is
  // what let them drift to three times the reference's transparency at five times
  // its blur.
  "before:rounded-[inherit] before:[backdrop-filter:var(--floating-backdrop)]",
].join(" ");

/**
 * How a surface driven by a Base UI part arrives and leaves.
 *
 * A transition, not the `rise-in` keyframe, and the difference is the leaving. Base UI
 * marks a closing part `data-ending-style` and holds it mounted until its animations
 * finish, so a panel can withdraw the way it came — where before every popover, menu,
 * tooltip and dialog in the app rose in politely and then blinked out of existence, an
 * asymmetry you feel as the UI being faster to discard your attention than to earn it.
 *
 * Interruptible too, which a keyframe is not: reopen a panel mid-dismiss and a
 * transition continues from wherever it had got to, while a keyframe restarts from its
 * own first frame — visibly, on the double-click that opens a menu you just closed.
 *
 * Leaving is quicker than arriving. An entrance introduces something and can afford to
 * be watched; a dismissal answers a click that already happened, and giving the two the
 * same duration makes the pointer feel like it is waiting for the interface to keep up.
 *
 * `scale` and `translate` rather than `transform`: the positioner owns `transform` to
 * place the panel, and this rides the popup inside it — two properties that compose
 * instead of one they would have to share.
 */
export const FLOATING_MOTION = [
  "transition-[opacity,scale,translate] ease-[var(--ease-out)] duration-[var(--dur-fast)]",
  "data-[starting-style]:scale-[0.97] data-[starting-style]:translate-y-1",
  "data-[ending-style]:scale-[0.97] data-[ending-style]:translate-y-1",
  "data-[starting-style]:opacity-0 data-[ending-style]:opacity-0",
  "data-[ending-style]:duration-[var(--dur-instant)]",
].join(" ");

/**
 * The dim behind a modal. Fades with the surface it dims, on the same clock: a scrim
 * that appears in one frame under a panel that takes 150ms to arrive announces the
 * modal before the modal is there.
 */
export const MODAL_SCRIM = [
  "fixed inset-0 z-[var(--layer-modal)] bg-scrim",
  "transition-opacity ease-[var(--ease-out)] duration-[var(--dur-fast)]",
  "data-[starting-style]:opacity-0 data-[ending-style]:opacity-0",
  "data-[ending-style]:duration-[var(--dur-instant)]",
].join(" ");

/**
 * The layer an anchored floating primitive competes on. It goes on the POSITIONER,
 * never on the panel inside it.
 *
 * Base UI positions the portaled node with a `transform`, which makes it a stacking
 * context — a z-index on the panel is then scoped inside that box and settles nothing
 * outside it, while the positioner left at `auto` loses to any page element holding a
 * layer. That is how the dock's panel picker came to render entirely behind the panel
 * it was opened from: right rect, opacity 1, not a pixel painted.
 */
export const FLOATING_LAYER = "z-[var(--layer-floating)]";

/** Menus, popovers, command panels — anything holding rows the pointer travels. */
export const FLOATING_PANEL = `${FLOATING_SURFACE_BASE} ${FLOATING_MOTION} rounded-[var(--floating-panel-radius)]`;

/** Tooltips and hover cards. Tighter radius: they wrap a line or two, and a panel
 *  radius on something that small reads as a lozenge. */
export const FLOATING_TIP = `${FLOATING_SURFACE_BASE} ${FLOATING_MOTION} rounded-[var(--floating-tip-radius)]`;

/**
 * A floating panel with no behaviour of its own — for a surface the ring's popover,
 * menu and tooltip models cannot host, such as the composer's pickers, which are
 * anchored inline so the caret never leaves the textarea.
 *
 * Keeps the one-shot `rise-in` keyframe rather than `FLOATING_MOTION`: nothing marks
 * this element as starting or ending, because its caller mounts and unmounts it
 * directly. A transition with no state to transition FROM animates nothing at all, so
 * the keyframe is what an entrance means here — and an exit is not reachable without an
 * owner willing to keep the node alive long enough to play one.
 */
export function FloatingSurface({
  className,
  ...props
}: ComponentProps<"div"> & { className?: string }) {
  return (
    <div
      {...props}
      className={cn(
        FLOATING_SURFACE_BASE,
        "animate-rise-in rounded-[var(--floating-panel-radius)]",
        className,
      )}
    />
  );
}
