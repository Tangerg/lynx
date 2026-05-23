// Helper for the "theme as plugin" pattern.
//
// Every theme registers the same shape: a ThemeSpec carrying a tokens
// map of CSS-var values. Most of that map is identical across themes of
// the same scheme — the shadow ladder follows a fixed dark vs light
// policy (DESIGN.md §5), and CTAs default to accent-driven. Only the
// palette itself (surface ladder, ink, hairlines, semantic hues, accent)
// is truly unique per theme.
//
// `defineThemePlugin` removes that boilerplate: pass in the palette,
// get back a fully-formed PluginSpec ready to drop into the builtin
// manifest. Adding a new theme = ~30 lines of palette data, no
// per-theme rewriting of shadow CSS or registration ceremony.

import { definePlugin } from "@/plugins/sdk";
import type { PluginSpec } from "@/plugins/sdk";

// Dark-policy shadows — surface ladder + hairlines do the work for
// inner cards, so xs/sm/md collapse to none. Only --shadow-lg lifts the
// floating-overlay layer (palette, dialogs, toasts).
const DARK_SHADOWS: Record<string, string> = {
  "shadow-xs":     "none",
  "shadow-sm":     "none",
  "shadow-md":     "none",
  "shadow-lg":
    "inset 0 1px 0 rgba(255, 255, 255, 0.04), " +
    "0 1px 2px rgba(0, 0, 0, 0.40), " +
    "0 8px 16px -4px rgba(0, 0, 0, 0.50), " +
    "0 24px 32px -8px rgba(0, 0, 0, 0.60), " +
    "inset 0 0 0 1px var(--color-border)",
  "shadow-card":   "none",
  "shadow-dialog": "var(--shadow-lg)",
  "shadow-soft":   "none",
  "shadow-pop":    "var(--shadow-lg)",
  "shadow-glow":   "0 0 12px color-mix(in srgb, var(--color-accent) 50%, transparent)",
  "shadow-input-focus":
    "0 0 0 2px color-mix(in srgb, var(--color-accent) 30%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};

// Light-policy shadows — surfaces need real shadows to read as elevated,
// stacked ladder Vercel-style.
const LIGHT_SHADOWS: Record<string, string> = {
  "shadow-xs":     "0 1px 2px rgba(15, 15, 15, 0.04)",
  "shadow-sm":
    "0 1px 2px rgba(15, 15, 15, 0.04), " +
    "0 2px 6px rgba(15, 15, 15, 0.06)",
  "shadow-md":
    "0 2px 4px rgba(15, 15, 15, 0.04), " +
    "0 8px 20px rgba(15, 15, 15, 0.10)",
  "shadow-lg":
    "0 4px 12px rgba(15, 15, 15, 0.08), " +
    "0 24px 60px -12px rgba(15, 15, 15, 0.18)",
  "shadow-card":   "var(--shadow-sm)",
  "shadow-dialog": "var(--shadow-lg)",
  "shadow-pop":    "var(--shadow-lg)",
  "shadow-soft":   "var(--shadow-xs)",
  "shadow-glow":   "0 0 12px color-mix(in srgb, var(--color-accent) 40%, transparent)",
  "shadow-input-focus":
    "0 0 0 3px color-mix(in srgb, var(--color-accent) 14%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};

// Accent-driven CTA — the default for almost every theme. The Vercel
// signature (black-on-white CTA, accent reserved for live state) needs
// to override via spec.cta.
const ACCENT_CTA: Record<string, string> = {
  "color-cta":       "var(--color-accent)",
  "color-cta-hover": "var(--color-accent-border)",
  "color-cta-text":  "var(--color-text-on-accent)",
};

const SCHEME_ICON: Record<"dark" | "light", string> = {
  dark: "moon",
  light: "sun",
};

export type ThemePluginSpec = {
  /** Stable id — what `useUIStore.theme` persists. */
  id: string;
  /** User-facing label. */
  label: string;
  /** Drives shadow ladder choice + structural `theme-{scheme}` class. */
  scheme: "dark" | "light";
  /** Icon for the picker row. Defaults to moon/sun based on scheme. */
  icon?: string;
  /** Sort hint — lower comes first. */
  order?: number;
  /**
   * The theme-unique palette: surface ladder, ink, hairlines, semantic
   * hues, accent. Anything in here overrides the defaults the helper
   * supplies (shadows, CTA, depth-step).
   */
  palette: Record<string, string>;
  /**
   * Optional CTA overrides — themes that want non-accent-driven CTAs
   * (e.g. Vercel's signature black-on-white) supply them here.
   */
  cta?: Record<string, string>;
};

export function defineThemePlugin(spec: ThemePluginSpec): PluginSpec {
  const shadows = spec.scheme === "dark" ? DARK_SHADOWS : LIGHT_SHADOWS;
  const tokens: Record<string, string> = {
    "depth-step": "5%",
    ...ACCENT_CTA,
    ...shadows,
    // Palette comes after shadows so themes CAN override a shadow if
    // they need to, but rarely do.
    ...spec.palette,
    // CTA overrides last so they win over accent-driven defaults.
    ...(spec.cta ?? {}),
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
