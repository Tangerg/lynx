// Built-in plugin: Catppuccin Mocha theme.
//
// Canonical palette from catppuccin/catppuccin (Mocha — popular dark
// variant). Default accent = mauve, matching catppuccin/vscode.

import { defineThemePlugin } from "../themes/defineThemePlugin";

export default defineThemePlugin({
  id: "catppuccin-mocha",
  label: "Catppuccin Mocha",
  scheme: "dark",
  order: 40,
  palette: {
    // ---------- Brand — Catppuccin mauve ----------
    "color-accent":         "#cba6f7",
    "color-accent-border":  "#b18cf3",
    "color-accent-press":   "#9572ed",

    // ---------- Surface ladder — mantle / base ----------
    "color-bg":             "#181825",
    "color-surface":        "#1e1e2e",

    // ---------- Ink — text → subtext1 → subtext0 → overlay1 ----------
    "color-text":           "#cdd6f4",
    "color-text-bright":    "#ffffff",
    "color-text-soft":      "#bac2de", // subtext1
    "color-text-muted":     "#a6adc8", // subtext0
    "color-text-faint":     "#7f849c", // overlay1
    "color-text-on-accent": "#1e1e2e",

    // ---------- Hairlines — surface0/1/2 ladder ----------
    "color-border":         "#45475a", // surface1
    "color-border-soft":    "#585b70", // surface2
    "color-divider":        "#6c7086", // overlay0
    "color-app-divider":    "#45475a",

    // ---------- Semantic ----------
    "color-negative":       "#f38ba8",
    "color-warning":        "#fab387",
    "color-info":           "#89b4fa",
    "color-success":        "#a6e3a1",
  },
});
