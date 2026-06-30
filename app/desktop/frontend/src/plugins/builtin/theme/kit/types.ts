// Theme type surface — palette sections + override knobs consumed by
// `defineThemePlugin` and the token-builder. Lives in its own file so
// `tokens.ts` can import these without forming a cycle with
// `defineThemePlugin.ts` (which also imports token defaults from
// `tokens.ts`).

/** Single accent color + the ink that reads on top of it. The two
 *  derived shades (accentBorder for hover, accentPress for :active) are
 *  computed from `accent` via colord unless a theme passes explicit
 *  overrides — saves 20 hand-tuned hex values across the 10 builtins. */
export interface ThemeBrand {
  /** Primary accent. Used scarcely — live indicator, active tab line,
   *  focus ring, CTA fill (when CTA is accent-driven). */
  accent: string;
  /** Ink color that reads on top of an accent fill. Usually black on a
   *  light accent, white on a dark accent. */
  textOnAccent: string;
  /** Optional override — slightly darker than accent, used for hover
   *  borders / focus rings. Default: `colord(accent).darken(0.08)`. */
  accentBorder?: string;
  /** Optional override — two steps darker than accent, used for
   *  `:active` press states on CTAs. Default: `colord(accent).darken(0.16)`. */
  accentPress?: string;
}

/** Canvas (the outermost frame) + surface (the lifted panel). The
 *  surface-2 / surface-3 / surface-4 steps are derived by color-mix
 *  unless the theme overrides them — pass values here only if the
 *  scheme has its own non-linear ladder (e.g. Catppuccin's
 *  surface0/1/2/overlay0). */
export interface ThemeSurfaces {
  /** Page-level background. */
  bg: string;
  /** Default lifted surface — Panel, sidebar, message bubble. */
  surface: string;
  /** Optional explicit step 2 — hover/active row, command palette. */
  surface2?: string;
  /** Optional explicit step 3 — sub-nav, dropdown, popover. */
  surface3?: string;
  /** Optional explicit step 4 — deepest lifted surface. */
  surface4?: string;
}

/** The five-step ink ladder. Each step has a defined role — see
 *  DESIGN.md §2 for the hierarchy. */
export interface ThemeInk {
  /** Headlines + emphasized body. The anchor — the soft/muted/faint ramp
   *  derives from this when omitted. */
  text: string;
  /** True maximum-contrast text — pure white on dark, pure black on
   *  light. Used for h1-h3 and `<strong>`. */
  textBright: string;
  /** Body paragraph default. Omit to auto-derive as `text` at ~82% alpha
   *  (Apple-label style) — adapts to the surface behind it. Pin an explicit
   *  hue when the palette's ink ramp is intentional (Solarized, Catppuccin). */
  textSoft?: string;
  /** Secondary / inactive nav / meta. Omit to auto-derive (~56% alpha).
   *  Must clear WCAG AA at 11-12px. */
  textMuted?: string;
  /** Tertiary / disabled / footnotes. Omit to auto-derive (~38% alpha).
   *  Must clear WCAG AA at 11-12px on both canvas and surface. */
  textFaint?: string;
}

/** The three-step hairline ladder. DESIGN.md §2: use literal hex, not
 *  alpha-blended, so borders read as precise rather than approximate. */
export interface ThemeBorders {
  /** Default 1px border on cards / dividers / table rows. */
  border: string;
  /** Input focus border, emphasized divider. */
  borderSoft: string;
  /** Nested-surface borders, deeper contrast. */
  divider: string;
}

/** Four meaning-carrying colors. Used SPARINGLY per DESIGN.md §9 —
 *  never decoratively. */
export interface ThemeSemantic {
  /** Errors. RUN_ERROR banner, tool failure status, destructive CTA. */
  negative: string;
  /** User attention required. Approval card, waiting state dot. */
  warning: string;
  /** Inline links, info badges. */
  info: string;
  /** Run finished cleanly, action confirmed. NOT the brand accent —
   *  accent is "live", success is "finished cleanly". */
  success: string;
}

/** Primary CTA color trio. Defaults to accent-driven (most themes), but
 *  e.g. Lyra Light overrides this to pure black-on-white (Vercel
 *  signature) so the accent can stay reserved for "live" state. */
export interface ThemeCta {
  cta: string;
  ctaHover: string;
  ctaText: string;
}

/** Named shadow tokens.
 *  surface: quiet optical edge for cards and app chrome.
 *  composer: composer container only.
 *  popover: dropdowns, popovers, modals, command palette.
 *  focus: quiet focus ring — no glow halo.
 *  Override individual keys when a theme wants a different elevation language. */
export interface ThemeShadows {
  surface: string;
  composer: string;
  popover: string;
  focus: string;
}

/** Global radius scale — themes that want a sharper or rounder feel
 *  override the relevant tiers (e.g. brutalist themes set every radius
 *  to 0; Catppuccin-style themes might prefer 10/14 instead of 8/12). */
export interface ThemeRadii {
  xs: string;
  sm: string;
  md: string;
  lg: string;
  xl: string;
}

export interface ThemePluginSpec {
  /** Stable id — what `uiStore` persists to `lyra.ui`. */
  id: string;
  /** User-facing label. */
  label: string;
  /** Drives shadow ladder choice + structural `theme-{scheme}` class. */
  scheme: "dark" | "light";
  /** Icon for the picker row. Defaults to moon/sun based on scheme. */
  icon?: string;
  /** Sort hint — lower comes first. */
  order?: number;

  /** Required palette sections — TypeScript enforces full coverage. */
  brand: ThemeBrand;
  surfaces: ThemeSurfaces;
  ink: ThemeInk;
  borders: ThemeBorders;
  semantic: ThemeSemantic;

  /** Optional overrides — leave undefined to inherit scheme defaults. */
  cta?: Partial<ThemeCta>;
  shadows?: Partial<ThemeShadows>;
  radii?: Partial<ThemeRadii>;
  /**
   * Surface ladder step in percent (e.g. "5%"). Default = 5%.
   * Higher values give more contrast between surface / surface-2 / -3 /
   * -4, lower values flatten the ladder. Only effective when the theme
   * does NOT also supply explicit surface2/3/4 values.
   */
  depthStep?: string;

  /**
   * Escape hatch for any CSS variable not captured by the typed
   * sections — custom syntax tokens, theme-specific motion tweaks, etc.
   * Keys are CSS-variable names WITHOUT the leading `--`.
   */
  extras?: Record<string, string>;
}
