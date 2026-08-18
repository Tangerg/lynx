import type { CSSProperties } from "react";
import { CHAT_MIN_WIDTH_PX, DOCK_MIN_WIDTH_PX } from "@/lib/shellGeometry";

// How wide the context dock is.
//
// Custom properties on the row rather than a React style on the dock: the resizer
// writes the stored one on every pointer-move and the flank's geometry follows it in
// CSS with no render involved. The stored width only reaches React on release, so a
// drag costs one re-render instead of one per pointer event.
//
// A px width clamped against the row — the same model as the drawer — gives both
// edges of the card one sizing vocabulary across window resizes.
export const DOCK_WIDTH_PROPERTY = "--dock-width";

/**
 * The measure the flank actually takes: retain the dock's operable minimum, cap it at
 * half the row, and preserve the conversation floor whenever the row is wide enough —
 * `maxDockWidth`, spelled in CSS so a stored width also responds correctly when the
 * window changes size without a React render.
 *
 * The percentages are deliberately left unresolved. `var(--dock-width)` is substituted
 * where this is declared (the row, the only element that has it), while `50%` and
 * `100%` survive as tokens and resolve on the FLANK — against the row either way, since
 * it is both the dock's flex container and its containing block. That is what lets
 * globals.css spend one value on two properties whose percentages have to agree: the
 * column's basis, and the negative end margin that slides it out of the window.
 */
const DOCK_MEASURE_PROPERTY = "--dock-measure";
const DOCK_MEASURE = `max(${DOCK_MIN_WIDTH_PX}px, min(var(${DOCK_WIDTH_PROPERTY}), 50%, calc(100% - ${CHAT_MIN_WIDTH_PX}px)))`;

/** Row style carrying the dock's geometry: the width a drag starts from, and the
 *  measure the flank keeps whether it is showing or gone. */
export function dockWidthRow(width: number): CSSProperties {
  return {
    [DOCK_WIDTH_PROPERTY]: `${width}px`,
    [DOCK_MEASURE_PROPERTY]: DOCK_MEASURE,
  } as CSSProperties;
}
