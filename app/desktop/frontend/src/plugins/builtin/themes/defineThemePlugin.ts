// Helper for the "theme as plugin" pattern — turns a typed ThemePluginSpec
// into a PluginSpec ready for the builtin manifest. Required sections
// (brand / surfaces / ink / borders / semantic) are enforced by TypeScript;
// shadows / radii / depthStep / cta / extras are optional overrides.

import type { PluginSpec } from "@/plugins/sdk";
import { definePlugin } from "@/plugins/sdk";

// ---------- Typed palette sections ----------

/** Single accent color + the two derived shades the design system uses
 *  to give CTAs depth (hover border, pressed background) and the ink
 *  that reads cleanly on top of an accent fill. */
export interface ThemeBrand {
  /** Primary accent. Used scarcely — live indicator, active tab line,
   *  focus ring, CTA fill (when CTA is accent-driven). */
  accent: string;
  /** Slightly darker than accent — used for hover borders / focus rings
   *  where the accent itself would be too bright. */
  accentBorder: string;
  /** Two steps darker than accent — used for `:active` press states on
   *  CTA buttons so the user feels the click land. */
  accentPress: string;
  /** Ink color that reads on top of an accent fill. Usually black on a
   *  light accent, white on a dark accent. */
  textOnAccent: string;
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
  /** Headlines + emphasized body. */
  text: string;
  /** True maximum-contrast text — pure white on dark, pure black on
   *  light. Used for h1-h3 and `<strong>`. */
  textBright: string;
  /** Body paragraph default. */
  textSoft: string;
  /** Secondary / inactive nav / meta. Must clear WCAG AA at 11-12px. */
  textMuted: string;
  /** Tertiary / disabled / footnotes. Must clear WCAG AA at 11-12px on
   *  both canvas and surface. */
  textFaint: string;
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
  /** 1px gap between flush panels — usually = border. */
  appDivider: string;
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

// ---------- Optional override sections ----------

/** Primary CTA color trio. Defaults to accent-driven (most themes), but
 *  e.g. Lyra Light overrides this to pure black-on-white (Vercel
 *  signature) so the accent can stay reserved for "live" state. */
export interface ThemeCta {
  cta: string;
  ctaHover: string;
  ctaText: string;
}

/** Named shadow tokens. On dark, depth comes from surface ladder +
 *  hairlines — most tiers collapse to `none` and only the overlay
 *  layer (`shadow-lg`) gets a real stacked drop. On light, every tier
 *  carries a Vercel-style stacked shadow. Override individual tiers
 *  here when a theme wants a different elevation language (e.g. an
 *  ultra-flat theme might set lg to a single 1px line). */
export interface ThemeShadows {
  xs: string;
  sm: string;
  md: string;
  lg: string;
  card: string;
  dialog: string;
  pop: string;
  soft: string;
  glow: string;
  inputFocus: string;
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

// ---------- Default shadow + CTA ladders ----------

const DARK_SHADOWS: ThemeShadows = {
  xs: "none",
  sm: "none",
  md: "none",
  lg:
    "inset 0 1px 0 rgba(255, 255, 255, 0.04), " +
    "0 1px 2px rgba(0, 0, 0, 0.40), " +
    "0 8px 16px -4px rgba(0, 0, 0, 0.50), " +
    "0 24px 32px -8px rgba(0, 0, 0, 0.60), " +
    "inset 0 0 0 1px var(--color-border)",
  card: "none",
  dialog: "var(--shadow-lg)",
  pop: "var(--shadow-lg)",
  soft: "none",
  glow: "0 0 12px color-mix(in srgb, var(--color-accent) 50%, transparent)",
  inputFocus:
    "0 0 0 2px color-mix(in srgb, var(--color-accent) 30%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};

const LIGHT_SHADOWS: ThemeShadows = {
  xs: "0 1px 2px rgba(15, 15, 15, 0.04)",
  sm: "0 1px 2px rgba(15, 15, 15, 0.04), 0 2px 6px rgba(15, 15, 15, 0.06)",
  md: "0 2px 4px rgba(15, 15, 15, 0.04), 0 8px 20px rgba(15, 15, 15, 0.10)",
  lg: "0 4px 12px rgba(15, 15, 15, 0.08), 0 24px 60px -12px rgba(15, 15, 15, 0.18)",
  card: "var(--shadow-sm)",
  dialog: "var(--shadow-lg)",
  pop: "var(--shadow-lg)",
  soft: "var(--shadow-xs)",
  glow: "0 0 12px color-mix(in srgb, var(--color-accent) 40%, transparent)",
  inputFocus:
    "0 0 0 3px color-mix(in srgb, var(--color-accent) 14%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};

const DEFAULT_RADII: ThemeRadii = {
  xs: "4px",
  sm: "6px",
  md: "8px",
  lg: "12px",
  xl: "16px",
};

const SCHEME_ICON: Record<"dark" | "light", string> = {
  dark: "moon",
  light: "sun",
};

// ---------- The spec ----------

export interface ThemePluginSpec {
  /** Stable id — what `themeStore` persists to `lyra.theme`. */
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

// ---------- Helper that resolves spec → flat tokens map ----------

export function defineThemePlugin(spec: ThemePluginSpec): PluginSpec {
  const shadowDefaults = spec.scheme === "dark" ? DARK_SHADOWS : LIGHT_SHADOWS;
  const shadows: ThemeShadows = { ...shadowDefaults, ...spec.shadows };
  const radii: ThemeRadii = { ...DEFAULT_RADII, ...spec.radii };

  const ctaDefaults: ThemeCta = {
    cta: spec.brand.accent,
    ctaHover: spec.brand.accentBorder,
    ctaText: spec.brand.textOnAccent,
  };
  const cta: ThemeCta = { ...ctaDefaults, ...spec.cta };

  const tokens: Record<string, string> = {
    "depth-step": spec.depthStep ?? "5%",

    // Brand
    "color-accent": spec.brand.accent,
    "color-accent-border": spec.brand.accentBorder,
    "color-accent-press": spec.brand.accentPress,
    "color-text-on-accent": spec.brand.textOnAccent,

    // Surfaces — surface2/3/4 default to color-mix() in tokens.css; only
    // emit explicit values when the theme provided them.
    "color-bg": spec.surfaces.bg,
    "color-surface": spec.surfaces.surface,
    ...(spec.surfaces.surface2 ? { "color-surface-2": spec.surfaces.surface2 } : {}),
    ...(spec.surfaces.surface3 ? { "color-surface-3": spec.surfaces.surface3 } : {}),
    ...(spec.surfaces.surface4 ? { "color-surface-4": spec.surfaces.surface4 } : {}),

    // Ink
    "color-text": spec.ink.text,
    "color-text-bright": spec.ink.textBright,
    "color-text-soft": spec.ink.textSoft,
    "color-text-muted": spec.ink.textMuted,
    "color-text-faint": spec.ink.textFaint,

    // Borders
    "color-border": spec.borders.border,
    "color-border-soft": spec.borders.borderSoft,
    "color-divider": spec.borders.divider,
    "color-app-divider": spec.borders.appDivider,

    // Semantic
    "color-negative": spec.semantic.negative,
    "color-warning": spec.semantic.warning,
    "color-info": spec.semantic.info,
    "color-success": spec.semantic.success,

    // CTA
    "color-cta": cta.cta,
    "color-cta-hover": cta.ctaHover,
    "color-cta-text": cta.ctaText,

    // Shadows
    "shadow-xs": shadows.xs,
    "shadow-sm": shadows.sm,
    "shadow-md": shadows.md,
    "shadow-lg": shadows.lg,
    "shadow-card": shadows.card,
    "shadow-dialog": shadows.dialog,
    "shadow-pop": shadows.pop,
    "shadow-soft": shadows.soft,
    "shadow-glow": shadows.glow,
    "shadow-input-focus": shadows.inputFocus,

    // Radii
    "radius-xs": radii.xs,
    "radius-sm": radii.sm,
    "radius-md": radii.md,
    "radius-lg": radii.lg,
    "radius-xl": radii.xl,

    // Free-form extras (last → wins over any of the above if a theme
    // really wants to override one).
    ...(spec.extras ?? {}),
  };

  return definePlugin({
    name: `lyra.builtin.theme-${spec.id}`,
    version: "1.0.0",
    setup({ host }) {
      host.theme.registerTheme({
        id: spec.id,
        label: spec.label,
        scheme: spec.scheme,
        icon: spec.icon ?? SCHEME_ICON[spec.scheme],
        order: spec.order,
        tokens,
      });
    },
  });
}
