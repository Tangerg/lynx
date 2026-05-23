// Built-in plugin: Solarized Light theme.
//
// Mirror of Solarized Dark — same 8 accent hues, base-* ladder inverted.

import { defineThemePlugin } from "../themes/defineThemePlugin";

export default defineThemePlugin({
  id: "solarized-light",
  label: "Solarized Light",
  scheme: "light",
  order: 31,
  palette: {
    // ---------- Brand — Solarized blue (identical to Dark) ----------
    "color-accent":         "#268bd2",
    "color-accent-border":  "#1e6fa6",
    "color-accent-press":   "#155383",

    // ---------- Surface ladder — base2 / base3 ----------
    "color-bg":             "#eee8d5", // base2
    "color-surface":        "#fdf6e3", // base3

    // ---------- Ink — inverted base-* ladder ----------
    "color-text":           "#657b83", // base00 — body
    "color-text-bright":    "#002b36", // base03
    "color-text-soft":      "#586e75", // base01
    "color-text-muted":     "#93a1a1", // base1
    "color-text-faint":     "#b5b2a2", // derived for "very faint"
    "color-text-on-accent": "#fdf6e3",

    // ---------- Hairlines — derived (Solarized has no light borders) ----------
    "color-border":         "#ddd6c1",
    "color-border-soft":    "#c4bda4",
    "color-divider":        "#93a1a1",
    "color-app-divider":    "#eee8d5",

    // ---------- Semantic — identical to Solarized Dark ----------
    "color-negative":       "#dc322f",
    "color-warning":        "#cb4b16",
    "color-info":           "#2aa198",
    "color-success":        "#859900",
  },
});
