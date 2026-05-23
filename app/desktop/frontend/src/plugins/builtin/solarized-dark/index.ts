// Built-in plugin: Solarized Dark theme.
//
// Ethan Schoonover's Solarized. Dark and Light share the same 8 accent
// hues — only the base-* ladder inverts.

import { defineThemePlugin } from "../themes/defineThemePlugin";

export default defineThemePlugin({
  id: "solarized-dark",
  label: "Solarized Dark",
  scheme: "dark",
  order: 30,
  palette: {
    // ---------- Brand — Solarized blue ----------
    "color-accent":         "#268bd2",
    "color-accent-border":  "#1e6fa6",
    "color-accent-press":   "#155383",

    // ---------- Surface ladder — base03 / base02 ----------
    "color-bg":             "#002b36",
    "color-surface":        "#073642",

    // ---------- Ink — base01 → base00 → base0 → base1 → base3 ----------
    "color-text":           "#839496", // base0 — body
    "color-text-bright":    "#fdf6e3", // base3
    "color-text-soft":      "#93a1a1", // base1
    "color-text-muted":     "#657b83", // base00
    "color-text-faint":     "#586e75", // base01 — comments
    "color-text-on-accent": "#fdf6e3",

    // ---------- Hairlines ----------
    "color-border":         "#586e75", // base01
    "color-border-soft":    "#657b83", // base00
    "color-divider":        "#93a1a1", // base1
    "color-app-divider":    "#073642", // base02

    // ---------- Semantic — Solarized accent hues ----------
    "color-negative":       "#dc322f",
    "color-warning":        "#cb4b16",
    "color-info":           "#2aa198",
    "color-success":        "#859900",
  },
});
