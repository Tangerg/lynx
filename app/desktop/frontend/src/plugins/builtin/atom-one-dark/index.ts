// Built-in plugin: Atom One Dark theme.
//
// Canonical palette from the `one-dark-syntax` Atom package + VS Code's
// `One Dark Pro` extension (identical values across both). Accent =
// Atom's cursor blue, which doubles as the activity-bar marker.

import { defineThemePlugin } from "../themes/defineThemePlugin";

export default defineThemePlugin({
  id: "atom-one-dark",
  label: "Atom One Dark",
  scheme: "dark",
  order: 10,
  palette: {
    // ---------- Brand — Atom cursor blue ----------
    "color-accent":         "#528bff",
    "color-accent-border":  "#4078e6",
    "color-accent-press":   "#2f63cc",

    // ---------- Surface ladder ----------
    "color-bg":             "#1c2026", // panel background
    "color-surface":        "#282c34", // editor background

    // ---------- Ink — foreground → comment ladder ----------
    "color-text":           "#abb2bf",
    "color-text-bright":    "#ffffff",
    "color-text-soft":      "#abb2bf",
    "color-text-muted":     "#828997",
    "color-text-faint":     "#5c6370", // comment
    "color-text-on-accent": "#ffffff",

    // ---------- Hairlines — selection-derived ladder ----------
    "color-border":         "#3a3f4b",
    "color-border-soft":    "#4b5263",
    "color-divider":        "#5c6370",
    "color-app-divider":    "#3a3f4b",

    // ---------- Semantic — One Dark syntax ----------
    "color-negative":       "#e06c75",
    "color-warning":        "#d19a66",
    "color-info":           "#61afef",
    "color-success":        "#98c379",
  },
});
