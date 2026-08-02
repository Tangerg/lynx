// UI type ladder — every chrome text size in the app derives from ONE base size.
//
// Why a derived ladder instead of per-callsite pixel values: the app previously
// carried 16 distinct hardcoded `text-[Npx]` values across ~390 callsites, which
// is how a UI ends up with 11px and 11.5px text side by side. A ladder makes the
// steps enumerable, keeps them on a whole-pixel grid, and gives the user's base
// size preference a single place to act.
//
// The steps are absolute px, never rem: geometry (header height, row height,
// gutters) must hold still when the type base moves. A rem ladder drags every
// padding and width along with it, which makes fixed chrome heights impossible.
//
// Base 12. The Synara reference and Lynx baseline agree on a 9/10/11/12/13px
// chrome ladder, with reading text one step above the 12px base. The user may
// still move the base through the supported range; 12 is only the clean
// first-paint and preference fallback, not a second fixed-size path.

export const UI_FONT_SIZE_DEFAULT_PX = 14;
export const UI_FONT_SIZE_MIN_PX = 11;
export const UI_FONT_SIZE_MAX_PX = 18;

/** Ladder steps, small to large. `code` is the mono counterpart of `ui-sm`. */
export type UiTypeStep = "ui-2xs" | "ui-xs" | "ui-sm" | "ui-md" | "ui-lg" | "code";

export type UiTypeLadder = Readonly<Record<UiTypeStep, number>>;

// Ratio + floor per step. The ratios are chosen so a 12px base lands exactly on
// the 9/10/11/12/13 whole-pixel grid. The floors matter at the small end of the
// base range: ratio alone would sink `ui-2xs` to 8px at base 11, below the size
// Geist stays legible at. `ui-lg` floors at the base so it can never dip under it.
const STEPS: Readonly<Record<UiTypeStep, { readonly ratio: number; readonly floorPx: number }>> = {
  "ui-2xs": { ratio: 0.76, floorPx: 9 },
  "ui-xs": { ratio: 0.84, floorPx: 10 },
  "ui-sm": { ratio: 0.92, floorPx: 10 },
  "ui-md": { ratio: 1, floorPx: UI_FONT_SIZE_MIN_PX },
  "ui-lg": { ratio: 1.08, floorPx: 0 },
  code: { ratio: 0.95, floorPx: 10 },
};

// Headroom above the base range: `ui-lg` overshoots the base by 8%, so the cap
// has to sit above UI_FONT_SIZE_MAX_PX or the top of the ladder would flatten.
const CEILING_PX = UI_FONT_SIZE_MAX_PX + 2;

/** Clamps a user-supplied base size into the supported range. `null` = default. */
export function normalizeUiFontSize(value: number | null | undefined): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return UI_FONT_SIZE_DEFAULT_PX;
  return Math.min(UI_FONT_SIZE_MAX_PX, Math.max(UI_FONT_SIZE_MIN_PX, Math.round(value)));
}

/** Derives every ladder step from a base size. */
export function uiTypeLadder(basePx: number | null | undefined): UiTypeLadder {
  const base = normalizeUiFontSize(basePx);
  const ladder = {} as Record<UiTypeStep, number>;
  for (const step of Object.keys(STEPS) as UiTypeStep[]) {
    const { ratio, floorPx } = STEPS[step];
    const floor = Math.max(floorPx, step === "ui-lg" ? base : 0);
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
    "--fs-ui-lg": `${ladder["ui-lg"]}px`,
    "--fs-code": `${ladder.code}px`,
  };
}
