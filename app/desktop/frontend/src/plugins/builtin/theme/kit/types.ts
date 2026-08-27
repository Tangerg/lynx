// Theme type surface — palette sections + override knobs consumed by
// `defineColorThemePlugin` and the token-builder. Lives in its own file so
// `tokens.ts` can import these without forming a cycle with
// `defineColorThemePlugin.ts` (which also imports token defaults from
// `tokens.ts`).

import type { ThemeNeutralSteps } from "@/plugins/sdk";
import type { Scheme } from "@/lib/appearance";

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

/** The four surface anchors.
 *
 *  `bg` and `surface` are the two region materials. The surface-2 / -3 / -4 steps
 *  above `surface` are ALWAYS derived by color-mix off `--depth-step`: a theme
 *  cannot pin them, because the contrast preference drives that step and a pinned
 *  ladder would make the slider partially dead.
 *
 *  `elevated` and `sunken` are anchors rather than rungs on that ladder because
 *  the ladder walks one direction only — toward the ink. A card lifts AWAY from
 *  the ink on a light palette (white over off-white) and TOWARD it on a dark one,
 *  while a well recedes under both. One monotonic mix cannot say all three, which
 *  is how the card fill — spelled as a ladder step — came out grey on light. */
export interface ThemeSurfaces {
  /** Page-level background — the reading plane. */
  bg: string;
  /** Region chrome — the drawer, the dock, the bars that frame the plane. */
  surface: string;
  /** Card fill: a message, a tool card, the composer — anything that reads as an
   *  object placed on a region. Defaults to the first ladder step. */
  elevated?: string;
  /** Recessed well: code bodies, terminal panes, diff hunks, text fields,
   *  progress tracks. Cut INTO the surface in both schemes. Defaults to a fixed
   *  per-scheme neutral, deliberately off the ladder — a control's own fill must
   *  not drift when the contrast slider moves. */
  sunken?: string;
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
 *  e.g. ScopeApp Light overrides this to pure black-on-white (Vercel
 *  signature) so the accent can stay reserved for "live" state. */
export interface ThemeCta {
  cta: string;
  ctaHover: string;
  ctaText: string;
}

export interface ColorThemePluginSpec {
  /** Stable id — what `uiStore` persists to `scopeapp.ui`. */
  id: string;
  /** User-facing label. */
  label: string;
  /** Drives the structural `theme-{scheme}` class and scheme-aware assets. */
  scheme: Scheme;
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

  /**
   * Opt in to having the neutral family follow the LIVE accent.
   *
   * A theme that sets this declares where each neutral sits — its lightness, and the
   * chroma it carries at the reference accent — and the shell rewrites those onto
   * whatever accent the user picked (see `kit/accentTint`). The literals in `surfaces`
   * and `borders` stay as the first-paint values for the default accent.
   *
   * Palette themes must NOT set it: Solarized's base3 is Solarized, not a tint of
   * whatever accent happens to be selected.
   */
  neutralSteps?: ThemeNeutralSteps;

  /**
   * Escape hatch for palette variables not captured by the typed sections.
   * Geometry, elevation and motion belong to a visual-style contribution.
   * Keys are CSS-variable names WITHOUT the leading `--`.
   */
  extras?: Record<string, string>;
}
