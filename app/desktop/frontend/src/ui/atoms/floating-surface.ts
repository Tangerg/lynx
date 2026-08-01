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
// No border. The edge is the first layer of `--shadow-popover`, drawn from the same
// `--seam-line` hairline as the drawer seam and the composer, so every floating
// edge in the app is one value and each follows its own corner radius exactly.

const FLOATING_SURFACE_BASE = [
  "relative z-50 overflow-hidden text-fg",
  "bg-canvas/70 shadow-[var(--shadow-popover)] animate-rise-in",
  "before:pointer-events-none before:absolute before:inset-0 before:-z-1",
  "before:rounded-[inherit] before:backdrop-blur-2xl before:backdrop-saturate-150",
].join(" ");

/** Menus, popovers, command panels — anything holding rows the pointer travels. */
export const FLOATING_PANEL = `${FLOATING_SURFACE_BASE} rounded-[var(--floating-panel-radius)]`;

/** Tooltips and hover cards. Tighter radius: they wrap a line or two, and a panel
 *  radius on something that small reads as a lozenge. */
export const FLOATING_TIP = `${FLOATING_SURFACE_BASE} rounded-[var(--floating-tip-radius)]`;
