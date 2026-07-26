import type { CSSProperties } from "react";

// How wide the context dock is.
//
// A custom property on the row rather than a React style on the dock: the
// resizer writes it on every pointer-move and the dock's `flex-basis` follows it
// in CSS with no render involved. The stored width only reaches React on release,
// so a drag costs one re-render instead of one per pointer event.
//
// A px width, clamped against the row — the same model as the drawer. It used to
// be a fraction of the row, which meant the dock's width changed meaning as the
// window resized and the two edges of the card were sized by different ideas.
export const DOCK_WIDTH_PROPERTY = "--dock-width";

/** Row style carrying the persisted width — the drag's starting point. */
export function dockWidthRow(width: number): CSSProperties {
  return { [DOCK_WIDTH_PROPERTY]: `${width}px` } as CSSProperties;
}

/** Dock column style, constant: the width lives in the property above. */
export const DOCK_COLUMN: CSSProperties = { flexBasis: `var(${DOCK_WIDTH_PROPERTY})` };
