// Theme token computation — defaults ladder + spec → flat CSS-variable
// map. Split out of defineThemePlugin.ts so the entry shim there reads
// as a small registration wrapper, and so this pure-function workhorse
// (buildTokenMap) can be unit-tested independently of the plugin
// machinery.

import { colord } from "colord";
import type { ThemeCta, ThemePluginSpec, ThemeRadii, ThemeShadows } from "./types";

// ---------------------------------------------------------------------------
// Default shadow + radii ladders
// ---------------------------------------------------------------------------

export const DARK_SHADOWS: ThemeShadows = {
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
  glow: "0 0 12px color-mix(in oklab, var(--color-accent) 50%, transparent)",
  inputFocus:
    "0 0 0 2px color-mix(in oklab, var(--color-accent) 30%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};

// Light-mode elevation uses the three-layer umbra / penumbra / ambient model
// (Material 3 / Ant) instead of a single drop: a tight near-opaque contact
// shadow, a medium fill, and a wide faint ambient layer. Three soft layers read
// as real diffused light; one hard layer reads as a sticker. Tuned restrained
// (neutral `#0f0f0f`, low alpha) to stay on the Vercel side of subtle.
export const LIGHT_SHADOWS: ThemeShadows = {
  xs: "0 1px 1px -0.5px rgba(15, 15, 15, 0.04), 0 1px 2px -1px rgba(15, 15, 15, 0.05)",
  sm:
    "0 1px 1px -0.5px rgba(15, 15, 15, 0.05), " +
    "0 2px 4px -1px rgba(15, 15, 15, 0.06), " +
    "0 4px 8px -2px rgba(15, 15, 15, 0.05)",
  md:
    "0 1px 2px -0.5px rgba(15, 15, 15, 0.06), " +
    "0 4px 8px -2px rgba(15, 15, 15, 0.07), " +
    "0 12px 20px -4px rgba(15, 15, 15, 0.08)",
  lg:
    "0 1px 3px -0.5px rgba(15, 15, 15, 0.07), " +
    "0 8px 16px -4px rgba(15, 15, 15, 0.10), " +
    "0 24px 44px -12px rgba(15, 15, 15, 0.14)",
  card: "var(--shadow-sm)",
  dialog: "var(--shadow-lg)",
  pop: "var(--shadow-lg)",
  soft: "var(--shadow-xs)",
  glow: "0 0 12px color-mix(in oklab, var(--color-accent) 40%, transparent)",
  inputFocus:
    "0 0 0 3px color-mix(in oklab, var(--color-accent) 14%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};

export const DEFAULT_RADII: ThemeRadii = {
  xs: "4px",
  sm: "6px",
  md: "8px",
  lg: "12px",
  xl: "16px",
};

export const SCHEME_ICON: Record<"dark" | "light", string> = {
  dark: "moon",
  light: "sun",
};

// ---------------------------------------------------------------------------
// buildTokenMap — spec → flat CSS-variable map
// ---------------------------------------------------------------------------

/**
 * Build the flat CSS-variable map a theme registers as `tokens`. Pure
 * function — same input always produces the same output, no I/O.
 *
 * Resolution rules:
 *  - shadow defaults pick from DARK/LIGHT by `spec.scheme`; spec.shadows
 *    overrides per-key
 *  - radii defaults pick from DEFAULT_RADII; spec.radii overrides per-key
 *  - accentBorder / accentPress auto-derive from spec.brand.accent via
 *    colord unless the spec passes explicit overrides
 *  - CTA defaults to accent-driven (accent fill + textOnAccent ink);
 *    spec.cta overrides individual fields
 *  - surface2 / surface3 / surface4 emit only when explicitly provided —
 *    otherwise tokens.css color-mix() ladder kicks in
 *  - spec.extras wins on collision (last spread)
 */
export function buildTokenMap(spec: ThemePluginSpec): Record<string, string> {
  const shadowDefaults = spec.scheme === "dark" ? DARK_SHADOWS : LIGHT_SHADOWS;
  const shadows: ThemeShadows = { ...shadowDefaults, ...spec.shadows };
  const radii: ThemeRadii = { ...DEFAULT_RADII, ...spec.radii };

  // Auto-derive accentBorder / accentPress from the base accent via
  // colord. Themes can still pass explicit values when the perceptual
  // darkening doesn't land where the palette wants it.
  // Ink ramp fallback: `text` at reduced opacity, mixed over transparent so it
  // composites against whatever surface it sits on (Apple label adaptivity).
  const inkAlpha = (pct: number) => `color-mix(in oklab, var(--color-text) ${pct}%, transparent)`;

  const accent = colord(spec.brand.accent);
  const accentBorder = spec.brand.accentBorder ?? accent.darken(0.08).toHex();
  const accentPress = spec.brand.accentPress ?? accent.darken(0.16).toHex();
  const cta: ThemeCta = {
    cta: spec.brand.accent,
    ctaHover: accentBorder,
    ctaText: spec.brand.textOnAccent,
    ...spec.cta,
  };

  return {
    "depth-step": spec.depthStep ?? "5%",

    // Brand
    "color-accent": spec.brand.accent,
    "color-accent-border": accentBorder,
    "color-accent-press": accentPress,
    "color-text-on-accent": spec.brand.textOnAccent,

    // Surfaces — surface2/3/4 default to color-mix() in tokens.css; only
    // emit explicit values when the theme provided them.
    "color-bg": spec.surfaces.bg,
    "color-surface": spec.surfaces.surface,
    ...(spec.surfaces.surface2 ? { "color-surface-2": spec.surfaces.surface2 } : {}),
    ...(spec.surfaces.surface3 ? { "color-surface-3": spec.surfaces.surface3 } : {}),
    ...(spec.surfaces.surface4 ? { "color-surface-4": spec.surfaces.surface4 } : {}),

    // Ink — soft/muted/faint default to `text` at decreasing alpha (Apple
    // label model) so a theme can ship just `text` + `textBright` and get an
    // adaptive ramp; palette themes pin explicit hues to keep their identity.
    "color-text": spec.ink.text,
    "color-text-bright": spec.ink.textBright,
    "color-text-soft": spec.ink.textSoft ?? inkAlpha(82),
    "color-text-muted": spec.ink.textMuted ?? inkAlpha(56),
    "color-text-faint": spec.ink.textFaint ?? inkAlpha(38),

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

    // Free-form extras win on collision so theme-level overrides
    // always take precedence.
    ...spec.extras,
  };
}
