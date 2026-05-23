// Built-in plugin: Atom One Light theme.
//
// Canonical palette from the `one-light-syntax` Atom package + VS Code's
// `Atom One Light` theme.

import { defineThemePlugin } from "../themes/defineThemePlugin";

export default defineThemePlugin({
  id: "atom-one-light",
  label: "Atom One Light",
  scheme: "light",
  order: 11,
  palette: {
    // ---------- Brand — One Light cursor blue ----------
    "color-accent":         "#526fff",
    "color-accent-border":  "#4060e8",
    "color-accent-press":   "#2d4ac6",

    // ---------- Surface ladder ----------
    "color-bg":             "#f0f0f1", // sidebar/panel
    "color-surface":        "#fafafa", // editor

    // ---------- Ink ----------
    "color-text":           "#383a42",
    "color-text-bright":    "#000000",
    "color-text-soft":      "#383a42",
    "color-text-muted":     "#696c77",
    "color-text-faint":     "#a0a1a7", // comment
    "color-text-on-accent": "#ffffff",

    // ---------- Hairlines — selection band ----------
    "color-border":         "#e5e5e6",
    "color-border-soft":    "#d4d4d6",
    "color-divider":        "#a0a1a7",
    "color-app-divider":    "#e5e5e6",

    // ---------- Semantic — One Light syntax ----------
    "color-negative":       "#e45649",
    "color-warning":        "#986801",
    "color-info":           "#4078f2",
    "color-success":        "#50a14f",
  },
});
