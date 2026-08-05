import type { ComponentProps } from "react";
import { cn } from "@/lib/classNames";

// The shell every floating panel wears: frosted fill, one shadow-drawn edge, one
// depth. Extracted because menus, popovers and tooltips had three byte-identical
// copies of it and the last two changes to it — adding the blur, then removing the
// border that doubled its edge — had to be made three times.
//
// Frosted rather than opaque: a panel that lets a hint of what it covers show
// through reads as glass above the surface, where a solid fill reads as a second
// page pasted over it. The blur sits on a `before` layer so it composites UNDER the
// content instead of blurring it.
//
// No border. The edge is the first layer of `--shadow-popover` — a half-pixel ring
// drawn from `--seam-line`, so every floating edge follows the same contrast algorithm
// as region boundaries and its own corner radius exactly. That claim was aspirational
// until now: the token existed, this comment described it, and the popover's shadow had
// exactly one layer and it was a drop.

const FLOATING_SURFACE_BASE = [
  "relative z-50 overflow-hidden text-fg",
  "bg-[var(--app-floating-surface)] shadow-[var(--shadow-popover)] animate-rise-in",
  "before:pointer-events-none before:absolute before:inset-0 before:-z-1",
  "before:rounded-[inherit] before:backdrop-blur-2xl before:backdrop-saturate-150",
].join(" ");

/**
 * The layer a floating primitive competes on, and it goes on the POSITIONER — not
 * on the panel inside it.
 *
 * Base UI positions the portaled node with a `transform`, which makes it a
 * stacking context: the panel's own `z-50` is then scoped INSIDE it and settles
 * nothing outside. With the positioner left at `auto` the whole popup stacks by
 * DOM order and loses to any page element that owns a layer — the composer's
 * `relative z-[2]` when the model picker opens upward, and the Context Dock's
 * `z-15` backing, which is where the dock's own panel picker rendered fully
 * behind the panel it was opened from: present in the DOM, correct rect, opacity
 * 1, and not a pixel of it painted.
 *
 * Named here rather than spelled at each positioner because it had been spelled
 * at two of the three and the third was the bug. A floating primitive added
 * later gets the layer by reaching for this, not by remembering.
 */
export const FLOATING_LAYER = "z-50";

/** Menus, popovers, command panels — anything holding rows the pointer travels. */
export const FLOATING_PANEL = `${FLOATING_SURFACE_BASE} rounded-[var(--floating-panel-radius)]`;

/** Tooltips and hover cards. Tighter radius: they wrap a line or two, and a panel
 *  radius on something that small reads as a lozenge. */
export const FLOATING_TIP = `${FLOATING_SURFACE_BASE} rounded-[var(--floating-tip-radius)]`;

/**
 * A floating panel with no behaviour of its own.
 *
 * `DropdownMenu.Content`, `Popover` and `Tooltip` each wear the material above through
 * their own behaviour model. Three surfaces in the app have a behaviour model the ring
 * does not provide — the composer's @-file and slash pickers are anchored inline so the
 * caret never leaves the textarea, and the command palette is cmdk's — so all three had
 * spelled a panel out themselves. None of the three matched: each used `bg-canvas`
 * rather than the frosted fill, so they were opaque where every other floating thing in
 * the app is glass, and between them they picked two radii, neither of which was the
 * floating-panel token.
 */
export function FloatingSurface({
  className,
  ...props
}: ComponentProps<"div"> & { className?: string }) {
  return <div {...props} className={cn(FLOATING_PANEL, className)} />;
}
