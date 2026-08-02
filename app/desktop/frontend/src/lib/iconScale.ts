// Icon scale — every glyph in the app is one of five sizes, derived from the same
// base the type ladder uses.
//
// Why a ladder rather than a number at the call site: the app had drifted to
// eleven icon sizes (10 through 30) and four stroke weights. Lucide draws on a
// 24-unit grid, so a stroke of 2 renders at `2 × size / 24` on screen — which put
// the app's on-screen stroke anywhere between 0.83px and 1.5px. Glyphs that differ
// by 80% in weight read as several icon sets pasted together, and the fractional
// widths blur on top of it.
//
// Two rules fix both halves. Sizes come from the type base, so a glyph beside a
// label grows when the label does. Stroke is derived per size to hold ONE
// on-screen weight, which is the opposite of Lucide's own constant-ratio default:
// a ratio keeps small icons proportionally heavy, and at chrome sizes that reads
// as blobby rather than delicate.

import { normalizeUiFontSize } from "./typography";

/** `sm` is the default: a glyph beside body text. */
export type IconSize = "xs" | "sm" | "md" | "lg" | "xl";

export const ICON_SIZES: readonly IconSize[] = ["xs", "sm", "md", "lg", "xl"];

// Offsets from the type base, in px. `xl` is the only multiplier: display glyphs
// (empty states, avatars) scale with the whole surface rather than tracking a
// label two steps away.
const OFFSETS: Readonly<Record<Exclude<IconSize, "xl">, number>> = {
  xs: -2,
  sm: 0,
  md: 2,
  lg: 6,
};
const XL_RATIO = 2;

/** The on-screen stroke every glyph renders at, in CSS px. */
const STROKE_PX = 1.1;
// Lucide's geometry is drawn for a 2-unit stroke and its counters close up below
// about 1.25, so the derived width is held inside the range the artwork survives.
const STROKE_MIN = 1.25;
const STROKE_MAX = 2;

export function iconSizePx(size: IconSize, basePx: number | null | undefined): number {
  const base = normalizeUiFontSize(basePx);
  return size === "xl" ? Math.round(base * XL_RATIO) : base + OFFSETS[size];
}

/** Stroke width in Lucide's 24-unit viewBox that renders as [STROKE_PX] on screen. */
export function iconStrokeWidth(sizePx: number): number {
  const derived = (24 * STROKE_PX) / sizePx;
  return Math.round(Math.min(STROKE_MAX, Math.max(STROKE_MIN, derived)) * 100) / 100;
}

/** The ladder as the custom properties `ui/icons/icon.tsx` reads. */
export function iconScaleCssVariables(
  basePx: number | null | undefined,
): Readonly<Record<string, string>> {
  const variables: Record<string, string> = {};
  for (const size of ICON_SIZES) {
    const px = iconSizePx(size, basePx);
    variables[`--icon-${size}`] = `${px}px`;
    variables[`--icon-stroke-${size}`] = String(iconStrokeWidth(px));
  }
  return variables;
}
