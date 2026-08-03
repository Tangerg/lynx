// UI density — how much air the chrome gets.
//
// A separate axis from the type ladder (lib/typography.ts) on purpose: type size
// is about legibility, density is about how many rows fit on screen, and people
// want them independently. Scaling type to get a denser list makes the list
// unreadable; scaling rows leaves the text alone.
//
// Chrome-bar heights deliberately do NOT scale. The content header, the drawer
// header and the macOS traffic-light gutter are all pinned to one number
// (`--surface-header-height`) so they line up across the seam — a density pick
// must not be able to break that alignment.

export const UI_DENSITY_MODES = ["compact", "comfortable", "spacious"] as const;
export type UiDensity = (typeof UI_DENSITY_MODES)[number];

export const DEFAULT_UI_DENSITY: UiDensity = "comfortable";

const SCALE: Readonly<Record<UiDensity, number>> = {
  compact: 0.85,
  comfortable: 1,
  spacious: 1.15,
};

/** Comfortable-mode base values, in px. Every mode is these times its scale. */
const BASE_PX = {
  rowHeight: 38,
  rowGap: 10,
  navigationGutter: 12,
  navigationSectionGap: 22,
  navigationGroupGap: 12,
  columnGutter: 12,
  columnGutterWide: 20,
  composerEditorTop: 12,
  composerEditorBottom: 8,
  composerEditorStart: 12,
  composerEditorEnd: 14,
  composerFooter: 6,
  composerFooterEnd: 8,
} as const;

export function isUiDensity(value: unknown): value is UiDensity {
  return typeof value === "string" && (UI_DENSITY_MODES as readonly string[]).includes(value);
}

export function normalizeUiDensity(value: unknown): UiDensity {
  return isUiDensity(value) ? value : DEFAULT_UI_DENSITY;
}

/**
 * The mode as the `--density-*` custom properties the chrome reads. Names are
 * spelled out so a grep for a token finds both its writer and its readers.
 */
export function densityCssVariables(mode: unknown): Readonly<Record<string, string>> {
  const scale = SCALE[normalizeUiDensity(mode)];
  const px = (base: number) => `${Math.round(base * scale)}px`;
  return {
    "--density-row-height": px(BASE_PX.rowHeight),
    "--density-row-gap": px(BASE_PX.rowGap),
    "--density-navigation-gutter": px(BASE_PX.navigationGutter),
    "--density-navigation-section-gap": px(BASE_PX.navigationSectionGap),
    "--density-navigation-group-gap": px(BASE_PX.navigationGroupGap),
    "--density-column-gutter": px(BASE_PX.columnGutter),
    "--density-column-gutter-wide": px(BASE_PX.columnGutterWide),
    "--density-composer-editor-top": px(BASE_PX.composerEditorTop),
    "--density-composer-editor-bottom": px(BASE_PX.composerEditorBottom),
    "--density-composer-editor-start": px(BASE_PX.composerEditorStart),
    "--density-composer-editor-end": px(BASE_PX.composerEditorEnd),
    "--density-composer-footer": px(BASE_PX.composerFooter),
    "--density-composer-footer-end": px(BASE_PX.composerFooterEnd),
  };
}
