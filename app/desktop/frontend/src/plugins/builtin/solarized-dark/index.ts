// Built-in plugin: Solarized Dark theme.
//
// Ethan Schoonover's Solarized — the OG palette. 16 fixed colors;
// dark/light only swap which end of the base-* ladder is bg vs fg.
// All eight accent hues are identical between Dark and Light, which is
// what makes Solarized Solarized.

import { definePlugin } from "@/plugins/sdk";

const tokens: Record<string, string> = {
  // ---------- Brand — Solarized blue ----------
  "color-accent":         "#268bd2",
  "color-accent-border":  "#1e6fa6",
  "color-accent-press":   "#155383",

  // ---------- Surface ladder ----------
  // base03 (#002b36) = canvas, base02 (#073642) = "background highlights" = surface.
  "color-bg":             "#002b36",
  "color-surface":        "#073642",
  "depth-step":           "5%",

  // ---------- Ink ----------
  // Solarized's ladder: base01 (faint) → base00 → base0 (body) → base1 (emphasized) → base3 (bright)
  "color-text":           "#839496", // base0 — default body
  "color-text-bright":    "#fdf6e3", // base3
  "color-text-soft":      "#93a1a1", // base1 — emphasized content
  "color-text-muted":     "#657b83", // base00
  "color-text-faint":     "#586e75", // base01 — comments/secondary
  "color-text-on-accent": "#fdf6e3", // base3 on the blue accent

  // ---------- Hairlines ----------
  "color-border":         "#586e75", // base01
  "color-border-soft":    "#657b83", // base00
  "color-divider":        "#93a1a1", // base1
  "color-app-divider":    "#073642", // base02

  // ---------- Semantic — Solarized accent hues ----------
  "color-negative":       "#dc322f", // red
  "color-warning":        "#cb4b16", // orange
  "color-info":           "#2aa198", // cyan
  "color-success":        "#859900", // green

  // ---------- CTA ----------
  "color-cta":            "var(--color-accent)",
  "color-cta-hover":      "var(--color-accent-border)",
  "color-cta-text":       "var(--color-text-on-accent)",

  // ---------- Shadows (dark policy) ----------
  "shadow-xs":            "none",
  "shadow-sm":            "none",
  "shadow-md":            "none",
  "shadow-lg":
    "inset 0 1px 0 rgba(255, 255, 255, 0.04), " +
    "0 1px 2px rgba(0, 0, 0, 0.40), " +
    "0 8px 16px -4px rgba(0, 0, 0, 0.50), " +
    "0 24px 32px -8px rgba(0, 0, 0, 0.60), " +
    "inset 0 0 0 1px var(--color-border)",
  "shadow-card":          "none",
  "shadow-dialog":        "var(--shadow-lg)",
  "shadow-soft":          "none",
  "shadow-pop":           "var(--shadow-lg)",
  "shadow-glow":          "0 0 12px color-mix(in srgb, var(--color-accent) 50%, transparent)",
  "shadow-input-focus":
    "0 0 0 2px color-mix(in srgb, var(--color-accent) 30%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};

export default definePlugin({
  name: "lyra.builtin.theme-solarized-dark",
  version: "1.0.0",
  setup({ host }) {
    host.theme.registerTheme({
      id: "solarized-dark",
      label: "Solarized Dark",
      scheme: "dark",
      icon: "moon",
      order: 30,
      tokens,
    });
  },
});
