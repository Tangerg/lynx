// Built-in plugin: Tokyo Night Light theme.
//
// Canonical palette from enkia/tokyonight (Day/Light variant) —
// intentionally muted compared to other light themes.

import { defineThemePlugin } from "../themes/defineThemePlugin";

export default defineThemePlugin({
  id: "tokyo-night-light",
  label: "Tokyo Night Light",
  scheme: "light",
  order: 21,
  palette: {
    // ---------- Brand — Tokyo Night Day's darker blue ----------
    "color-accent":         "#34548a",
    "color-accent-border":  "#2a4373",
    "color-accent-press":   "#1f335a",

    // ---------- Surface ladder ----------
    "color-bg":             "#cbccd1", // bg_dark
    "color-surface":        "#d5d6db", // bg

    // ---------- Ink ----------
    "color-text":           "#343b58", // fg
    "color-text-bright":    "#1a1b26",
    "color-text-soft":      "#343b58",
    "color-text-muted":     "#4c505e", // fg_dark
    "color-text-faint":     "#848cb5", // comment
    "color-text-on-accent": "#ffffff",

    // ---------- Hairlines ----------
    "color-border":         "#a8aecb", // fg_gutter
    "color-border-soft":    "#c4c8d8",
    "color-divider":        "#848cb5",
    "color-app-divider":    "#a8aecb",

    // ---------- Semantic — Tokyo Night Day syntax ----------
    "color-negative":       "#8c4351",
    "color-warning":        "#965027",
    "color-info":           "#0f4b6e",
    "color-success":        "#485e30",
  },
});
