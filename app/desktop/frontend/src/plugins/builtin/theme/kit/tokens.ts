// Theme token computation — defaults ladder + spec → flat CSS-variable
// map. Split out of defineThemePlugin.ts so the entry module reads
// as a small registration wrapper, and so this pure-function workhorse
// (buildTokenMap) can be unit-tested independently of the plugin
// machinery.

import { colord } from "colord";
import type { ThemeCta, ThemePluginSpec, ThemeRadii, ThemeShadows } from "./types";

// Default shadow + radii ladders

// Elevation shadows follow DESKTOP_UI_POLISH.md §1's three-layer model:
//   1. edge ring — a 1px optical boundary as the FIRST layer (the floating
//      surface's edge; a shadow-ring, NOT a cheap CSS border);
//   2. contact — a tiny near shadow (surface touches the background);
//   3. ambient — a wide low-alpha falloff (elevation without a muddy border).
// Reserved for genuinely-floating elements only (JetBrains model): tiled/docked
// regions + in-flow cards separate by surface-ladder background delta, never by
// shadow — so there is no `surface` card shadow, only composer (the input that
// floats over the scrolling stream) + popover (dropdowns / palette / dialogs).
// On dark the edge ring is white and the depth layers near-black (a light
// ambient can't show on a dark canvas); on light, the doc's slate values.
// EXCEPTION — the composer is deliberately RINGLESS (no edge ring, ever, incl.
// on focus): it's separated from the canvas by its bg-surface fill (colour
// delta) + a soft diffuse shadow (the ChatGPT/Codex composer reference). Every
// ring on it read as a cheap border. Only `popover` carries the doc's ring.
export const DARK_SHADOWS: ThemeShadows = {
  composer: "0 2px 8px rgb(0 0 0 / 0.35), 0 16px 40px -10px rgb(0 0 0 / 0.55)",
  popover:
    "0 0 0 1px rgb(255 255 255 / 0.08), 0 1px 2px rgb(0 0 0 / 0.4), 0 8px 24px rgb(0 0 0 / 0.5)",
  // Geist two-layer focus ring: 2px gap in surface color + 2px accent.
  focus: "0 0 0 2px var(--color-bg), 0 0 0 4px var(--color-accent)",
};

export const LIGHT_SHADOWS: ThemeShadows = {
  composer: "0 2px 8px rgb(15 23 42 / 0.05), 0 12px 32px -8px rgb(15 23 42 / 0.14)",
  popover:
    "0 0 0 1px rgb(15 23 42 / 0.08), 0 1px 2px rgb(15 23 42 / 0.06), 0 8px 24px rgb(15 23 42 / 0.1)",
  focus: "0 0 0 2px var(--color-bg), 0 0 0 4px var(--color-accent)",
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

// buildTokenMap — spec → flat CSS-variable map

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

    // Semantic
    "color-negative": spec.semantic.negative,
    "color-warning": spec.semantic.warning,
    "color-info": spec.semantic.info,
    "color-success": spec.semantic.success,

    // CTA
    "color-cta": cta.cta,
    "color-cta-hover": cta.ctaHover,
    "color-cta-text": cta.ctaText,

    // Shadows — floating-elevation tokens only (no card `surface` shadow).
    "shadow-composer": shadows.composer,
    "shadow-popover": shadows.popover,
    "shadow-focus": shadows.focus,

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
