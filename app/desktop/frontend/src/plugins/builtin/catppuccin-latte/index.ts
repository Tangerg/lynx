// Built-in plugin: Catppuccin Latte theme.
//
// The light counterpart to Mocha. Latte's mauve is far more saturated
// than Mocha's — it has to bite against the bright surface.

import { defineThemePlugin } from "../themes/defineThemePlugin";

export default defineThemePlugin({
  id: "catppuccin-latte",
  label: "Catppuccin Latte",
  scheme: "light",
  order: 41,
  palette: {
    // ---------- Brand — Latte's saturated mauve ----------
    "color-accent":         "#8839ef",
    "color-accent-border":  "#6f25d4",
    "color-accent-press":   "#5817b3",

    // ---------- Surface ladder — mantle / base ----------
    "color-bg":             "#e6e9ef",
    "color-surface":        "#eff1f5",

    // ---------- Ink ----------
    "color-text":           "#4c4f69",
    "color-text-bright":    "#000000",
    "color-text-soft":      "#5c5f77", // subtext1
    "color-text-muted":     "#6c6f85", // subtext0
    "color-text-faint":     "#8c8fa1", // overlay1
    "color-text-on-accent": "#ffffff",

    // ---------- Hairlines — surface0/1/2 ----------
    "color-border":         "#ccd0da", // surface0
    "color-border-soft":    "#bcc0cc", // surface1
    "color-divider":        "#acb0be", // surface2
    "color-app-divider":    "#ccd0da",

    // ---------- Semantic ----------
    "color-negative":       "#d20f39",
    "color-warning":        "#fe640b",
    "color-info":           "#1e66f5",
    "color-success":        "#40a02b",
  },
});
