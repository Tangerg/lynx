// Theme token computation — defaults ladder + spec → flat CSS-variable
// map. Split out of defineColorThemePlugin.ts so the entry module reads
// as a small registration wrapper, and so this pure-function workhorse
// (buildTokenMap) can be unit-tested independently of the plugin
// machinery.

import { colord } from "colord";
import type { Scheme } from "@/lib/appearance";
import type { ColorThemePluginSpec, ThemeCta } from "./types";

export const SCHEME_ICON: Record<Scheme, string> = {
  dark: "moon",
  light: "sun",
};

/** Recessed-well fallback for a theme that does not name its own. A fixed
 *  neutral rather than a ladder rung: a text field's fill is the control's own
 *  material and must hold still while the contrast slider moves the regions. */
const SCHEME_SUNKEN: Record<Scheme, string> = {
  dark: "#1c1c21",
  light: "#f1f1f4",
};

// buildTokenMap — spec → flat CSS-variable map

/**
 * Build the flat CSS-variable map a theme registers as `tokens`. Pure
 * function — same input always produces the same output, no I/O.
 *
 * Resolution rules:
 *  - accentBorder / accentPress auto-derive from spec.brand.accent via
 *    colord unless the spec passes explicit overrides
 *  - CTA defaults to accent-driven (accent fill + textOnAccent ink);
 *    spec.cta overrides individual fields
 *  - spec.extras wins on collision (last spread)
 */
export function buildTokenMap(spec: ColorThemePluginSpec): Record<string, string> {
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
    // Brand
    "color-accent": spec.brand.accent,
    "color-accent-border": accentBorder,
    "color-accent-press": accentPress,
    "color-text-on-accent": spec.brand.textOnAccent,

    // Surfaces — the -2/-3/-4 steps are the color-mix() ladder in globals.css,
    // never emitted here: they track --depth-step (the contrast preference).
    // `elevated` and `sunken` are anchors precisely because that ladder can only
    // walk toward the ink; see ThemeSurfaces for why one mix cannot say all three.
    "color-bg": spec.surfaces.bg,
    "color-surface": spec.surfaces.surface,
    "color-elevated": spec.surfaces.elevated ?? "var(--color-surface-2)",
    "color-sunken": spec.surfaces.sunken ?? SCHEME_SUNKEN[spec.scheme],

    // Ink — soft/muted default to `text` at decreasing alpha. Faint deliberately
    // shares the muted fallback: a third lower-opacity text rung fails AA on
    // ordinary light/dark canvases. Themes may pin a distinct faint hue, but the
    // generic fallback must remain readable without knowing the final surface.
    "color-text": spec.ink.text,
    "color-text-bright": spec.ink.textBright,
    "color-text-soft": spec.ink.textSoft ?? inkAlpha(82),
    "color-text-muted": spec.ink.textMuted ?? inkAlpha(56),
    "color-text-faint": spec.ink.textFaint ?? spec.ink.textMuted ?? inkAlpha(56),

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

    // Free-form extras win on collision so palette-level overrides
    // always take precedence.
    ...spec.extras,
  };
}
