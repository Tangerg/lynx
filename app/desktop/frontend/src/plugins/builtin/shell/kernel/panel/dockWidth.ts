import type { CSSProperties } from "react";
import { CHAT_MIN_WIDTH_PX, DOCK_MIN_WIDTH_PX } from "@/lib/shellGeometry";

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

/**
 * Dock column style, constant: the width lives in the property above.
 *
 * The CSS expression mirrors `maxDockWidth`: retain the dock's operable minimum,
 * cap it at half the row, and preserve the conversation floor whenever the row
 * is wide enough. Keeping the same safety rule in CSS means a stored width also
 * responds correctly when the window changes without a React render.
 */
export const DOCK_COLUMN: CSSProperties = {
  flexBasis: `max(${DOCK_MIN_WIDTH_PX}px, min(var(${DOCK_WIDTH_PROPERTY}), 50%, calc(100% - ${CHAT_MIN_WIDTH_PX}px)))`,
};

/**
 * The same column at zero, so opening and closing the dock is a width the reading
 * plane can follow.
 *
 * It used to be `display: none`, which is not a state a transition can leave or
 * arrive at: the dock appeared and vanished between two frames while the drawer on
 * the other side of the same window slid. Zero BASIS keeps the element in flow, which
 * is what gives the conversation column something to reflow against — the mirror of
 * the drawer's in-flow spacer.
 */
export const DOCK_COLUMN_COLLAPSED: CSSProperties = { flexBasis: "0px" };
