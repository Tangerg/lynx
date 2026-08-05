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
  "bg-[var(--app-floating-surface)] shadow-[var(--shadow-popover)] animate-rise-in",
  "before:pointer-events-none before:absolute before:inset-0 before:-z-1",
  "before:rounded-[inherit] before:backdrop-blur-2xl before:backdrop-saturate-150",
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
export const FLOATING_PANEL = `${FLOATING_SURFACE_BASE} rounded-[var(--floating-panel-radius)]`;

/** Tooltips and hover cards. Tighter radius: they wrap a line or two, and a panel
 *  radius on something that small reads as a lozenge. */
export const FLOATING_TIP = `${FLOATING_SURFACE_BASE} rounded-[var(--floating-tip-radius)]`;

/**
 * A floating panel with no behaviour of its own — for a surface the ring's popover,
 * menu and tooltip models cannot host, such as the composer's pickers, which are
 * anchored inline so the caret never leaves the textarea.
 */
export function FloatingSurface({
  className,
  ...props
}: ComponentProps<"div"> & { className?: string }) {
  return <div {...props} className={cn(FLOATING_PANEL, className)} />;
}
