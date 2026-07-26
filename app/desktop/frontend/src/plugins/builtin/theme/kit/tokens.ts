// Theme token computation — defaults ladder + spec → flat CSS-variable
// map. Split out of defineThemePlugin.ts so the entry module reads
// as a small registration wrapper, and so this pure-function workhorse
// (buildTokenMap) can be unit-tested independently of the plugin
// machinery.

import { colord } from "colord";
import type { Scheme } from "@/lib/appearance";
import type { ThemeCta, ThemePluginSpec, ThemeShadows } from "./types";

// Default shadow ladders.
//
// Floating surfaces use the desktop polish model: optical edge ring + contact
// shadow + ambient shadow. The first layer is a 0.5px shadow ring, not a CSS
// border, so it gives the crisp Raycast/Geist edge without adding grey layout
// chrome. Tiled/docked regions separate by the chrome hairline and structural
// hairlines; these shadows are reserved for composer and transient surfaces.
export const DARK_SHADOWS: ThemeShadows = {
  composer: "0 0 0 1px var(--seam-line), 0 8px 40px -12px rgb(0 0 0 / 0.4)",
  popover:
    "0 0 0 1px var(--seam-line), 0 12px 32px -12px rgb(0 0 0 / 0.55), 0 2px 6px -2px rgb(0 0 0 / 0.4)",
};

export const LIGHT_SHADOWS: ThemeShadows = {
  composer:
    "0 0 0 1px var(--seam-line), 0 6px 30px -8px color-mix(in srgb, var(--color-text) 9%, transparent)",
  popover:
    "0 0 0 1px var(--seam-line), 0 10px 30px -10px color-mix(in srgb, var(--color-text) 14%, transparent)",
};

export const SCHEME_ICON: Record<Scheme, string> = {
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
 *  - accentBorder / accentPress auto-derive from spec.brand.accent via
 *    colord unless the spec passes explicit overrides
 *  - CTA defaults to accent-driven (accent fill + textOnAccent ink);
 *    spec.cta overrides individual fields
 *  - spec.extras wins on collision (last spread)
 */
export function buildTokenMap(spec: ThemePluginSpec): Record<string, string> {
  const shadowDefaults = spec.scheme === "dark" ? DARK_SHADOWS : LIGHT_SHADOWS;
  const shadows: ThemeShadows = { ...shadowDefaults, ...spec.shadows };

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

    // Surfaces — the -2/-3/-4 steps are the color-mix() ladder in globals.css,
    // never emitted here: they track --depth-step (the contrast preference).
    "color-bg": spec.surfaces.bg,
    "color-surface": spec.surfaces.surface,

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

    // Radii

    // Free-form extras win on collision so theme-level overrides
    // always take precedence.
    ...spec.extras,
  };
}
