// UI type ladder — every chrome text size in the app derives from ONE base size.
//
// A derived ladder makes the supported steps enumerable, keeps them on a
// whole-pixel grid, and gives the user's base
// size preference a single place to act.
//
// The steps are absolute px, never rem: geometry (header height, row height,
// gutters) must hold still when the type base moves. A rem ladder drags every
// padding and width along with it, which makes fixed chrome heights impossible.
//
// Base 14. The ladder keeps dense chrome one or two steps below its body text
// while preserving a readable 14px default. The user may still move the base
// through the supported range; 14 is only the clean first-paint and preference
// fallback, not a second fixed-size path.
//
// Nothing sits between the base and `prose`: a one-pixel intermediate cannot
// distinguish a title from a chrome label. Neither reference has a step there
// (ChatGPT runs 11/12/14, Claude 10/11/12/13/14); both put titles in a separate
// editorial ladder, which is where ours belongs.

export const UI_FONT_SIZE_DEFAULT_PX = 14;
export const UI_FONT_SIZE_MIN_PX = 11;
export const UI_FONT_SIZE_MAX_PX = 18;

/**
 * Ladder steps, small to large.
 *
 * Two are named by ROLE rather than by size, because their size is not what makes
 * them a step: `code` is the mono counterpart of `ui-sm`, and `prose` is the one
 * step for continuous reading — message copy, the composer, markdown body.
 *
 * A runtime list and not only a type: `lib/classNames.ts` has to name every step
 * for Tailwind Merge, and a hand-kept copy there is a step that silently stops
 * applying the day someone adds one here.
 */
export const UI_TYPE_STEPS = ["ui-2xs", "ui-xs", "ui-sm", "ui-md", "prose", "code"] as const;

export type UiTypeStep = (typeof UI_TYPE_STEPS)[number];

export type UiTypeLadder = Readonly<Record<UiTypeStep, number>>;

// Ratio + floor per step. The ratios preserve the existing whole-pixel ladder
// across the supported base range. The floors matter at the small end of the
// base range: ratio alone would sink `ui-2xs` to 8px at base 11, below the size
// Geist stays legible at. `prose` floors at the base so it can never dip under the
// chrome it sits above.
//
// `prose` is 1.14 because that is where both desktop references put continuous
// reading text against the same 14px chrome: ChatGPT's chat body is 16-17px with
// a 14px UI base, Claude's `.prose` is 16px. The step exists at all because a
// transcript read at the size of the labels around it has no main and no
// secondary — which is the whole reason the chrome ladder stops at the base.
const STEPS: Readonly<Record<UiTypeStep, { readonly ratio: number; readonly floorPx: number }>> = {
  "ui-2xs": { ratio: 0.76, floorPx: 9 },
  "ui-xs": { ratio: 0.84, floorPx: 10 },
  "ui-sm": { ratio: 0.92, floorPx: 10 },
  "ui-md": { ratio: 1, floorPx: UI_FONT_SIZE_MIN_PX },
  prose: { ratio: 1.14, floorPx: 0 },
  code: { ratio: 0.95, floorPx: 10 },
};

// Headroom above the base range: `prose` overshoots the base by 14%, so the cap
// has to sit above UI_FONT_SIZE_MAX_PX or the top of the ladder would flatten.
const CEILING_PX = UI_FONT_SIZE_MAX_PX + 3;

/** Clamps a user-supplied base size into the supported range. `null` = default. */
export function normalizeUiFontSize(value: number | null | undefined): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return UI_FONT_SIZE_DEFAULT_PX;
  return Math.min(UI_FONT_SIZE_MAX_PX, Math.max(UI_FONT_SIZE_MIN_PX, Math.round(value)));
}

/** Derives every ladder step from a base size. */
export function uiTypeLadder(basePx: number | null | undefined): UiTypeLadder {
  const base = normalizeUiFontSize(basePx);
  const ladder = {} as Record<UiTypeStep, number>;
  for (const step of UI_TYPE_STEPS) {
    const { ratio, floorPx } = STEPS[step];
    const floor = Math.max(floorPx, ratio > 1 ? base : 0);
    ladder[step] = Math.min(CEILING_PX, Math.max(floor, Math.round(base * ratio)));
  }
  return ladder;
}

/**
 * The ladder as the `--fs-*` custom properties `globals.css` maps into the
 * `text-ui-*` / `text-code` utilities. Names are spelled out so a grep for a
 * token finds both its writer and its readers.
 */
export function uiTypeLadderCssVariables(
  basePx: number | null | undefined,
): Readonly<Record<string, string>> {
  const ladder = uiTypeLadder(basePx);
  return {
    "--fs-ui-2xs": `${ladder["ui-2xs"]}px`,
    "--fs-ui-xs": `${ladder["ui-xs"]}px`,
    "--fs-ui-sm": `${ladder["ui-sm"]}px`,
    "--fs-ui-md": `${ladder["ui-md"]}px`,
    "--fs-prose": `${ladder.prose}px`,
    "--fs-code": `${ladder.code}px`,
  };
}
