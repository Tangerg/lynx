// Built-in plugin: Solarized Light theme.
//
// Mirror of Solarized Dark — same 8 accent hues, base-* ladder inverted.
// base3 (#fdf6e3) is the editor; base2 (#eee8d5) is the "background
// highlights" tone we use for borders + canvas.

import { definePlugin } from "@/plugins/sdk";

const tokens: Record<string, string> = {
  // ---------- Brand — Solarized blue (same as Dark — that's the point) ----------
  "color-accent":         "#268bd2",
  "color-accent-border":  "#1e6fa6",
  "color-accent-press":   "#155383",

  // ---------- Surface ladder ----------
  "color-bg":             "#eee8d5", // base2 — canvas (background highlights)
  "color-surface":        "#fdf6e3", // base3 — editor
  "depth-step":           "5%",

  // ---------- Ink — inverted base-* ladder ----------
  "color-text":           "#657b83", // base00 — default body
  "color-text-bright":    "#002b36", // base03
  "color-text-soft":      "#586e75", // base01 — emphasized
  "color-text-muted":     "#93a1a1", // base1
  "color-text-faint":     "#b5b2a2", // derived between base1 and base2 for "very faint" ink
  "color-text-on-accent": "#fdf6e3", // base3 on the blue

  // ---------- Hairlines ----------
  // Solarized doesn't have explicit "light borders" — base2/base1 are
  // the closest the palette offers. Use base2 for default subtle borders
  // and a derived darker tone for soft borders.
  "color-border":         "#ddd6c1",
  "color-border-soft":    "#c4bda4",
  "color-divider":        "#93a1a1", // base1
  "color-app-divider":    "#eee8d5", // base2

  // ---------- Semantic — identical to Solarized Dark ----------
  "color-negative":       "#dc322f", // red
  "color-warning":        "#cb4b16", // orange
  "color-info":           "#2aa198", // cyan
  "color-success":        "#859900", // green

  // ---------- CTA ----------
  "color-cta":            "var(--color-accent)",
  "color-cta-hover":      "var(--color-accent-border)",
  "color-cta-text":       "var(--color-text-on-accent)",

  // ---------- Shadows (light policy) ----------
  "shadow-xs":            "0 1px 2px rgba(15, 15, 15, 0.04)",
  "shadow-sm":
    "0 1px 2px rgba(15, 15, 15, 0.04), " +
    "0 2px 6px rgba(15, 15, 15, 0.06)",
  "shadow-md":
    "0 2px 4px rgba(15, 15, 15, 0.04), " +
    "0 8px 20px rgba(15, 15, 15, 0.10)",
  "shadow-lg":
    "0 4px 12px rgba(15, 15, 15, 0.08), " +
    "0 24px 60px -12px rgba(15, 15, 15, 0.18)",
  "shadow-card":          "var(--shadow-sm)",
  "shadow-dialog":        "var(--shadow-lg)",
  "shadow-pop":           "var(--shadow-lg)",
  "shadow-soft":          "var(--shadow-xs)",
  "shadow-glow":          "0 0 12px color-mix(in srgb, var(--color-accent) 40%, transparent)",
  "shadow-input-focus":
    "0 0 0 3px color-mix(in srgb, var(--color-accent) 14%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};

export default definePlugin({
  name: "lyra.builtin.theme-solarized-light",
  version: "1.0.0",
  setup({ host }) {
    host.theme.registerTheme({
      id: "solarized-light",
      label: "Solarized Light",
      scheme: "light",
      icon: "sun",
      order: 31,
      tokens,
    });
  },
});
