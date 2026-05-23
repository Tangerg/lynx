// Built-in plugin: Tokyo Night Storm theme.
//
// Canonical palette from enkia/tokyonight (Storm variant) — same values
// VS Code's `Tokyo Night` extension and the Vim/Neovim port use.

import { defineThemePlugin } from "../themes/defineThemePlugin";

export default defineThemePlugin({
  id: "tokyo-night-storm",
  label: "Tokyo Night Storm",
  scheme: "dark",
  order: 20,
  palette: {
    // ---------- Brand — Tokyo Night signature blue ----------
    "color-accent":         "#7aa2f7",
    "color-accent-border":  "#5d86e6",
    "color-accent-press":   "#4068c5",

    // ---------- Surface ladder ----------
    "color-bg":             "#1f2335", // bg_dark
    "color-surface":        "#24283b", // bg

    // ---------- Ink ----------
    "color-text":           "#c0caf5", // fg
    "color-text-bright":    "#ffffff",
    "color-text-soft":      "#a9b1d6", // fg_dark
    "color-text-muted":     "#787c99",
    "color-text-faint":     "#565f89", // comment
    "color-text-on-accent": "#1a1b26",

    // ---------- Hairlines ----------
    "color-border":         "#3b4261", // fg_gutter
    "color-border-soft":    "#545c7e",
    "color-divider":        "#565f89",
    "color-app-divider":    "#3b4261",

    // ---------- Semantic — Tokyo Night syntax ----------
    "color-negative":       "#f7768e",
    "color-warning":        "#ff9e64",
    "color-info":           "#7dcfff",
    "color-success":        "#9ece6a",
  },
});
