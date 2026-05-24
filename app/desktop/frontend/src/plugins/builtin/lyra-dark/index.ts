// Built-in plugin: Lyra Dark — the system default theme.
//
// Synthesis of Linear (canvas / surface / hairline) + Vercel (Geist /
// elevation). The values here are the source of truth — the matching
// declarations in `styles/tokens.css` (`:root`) are first-paint
// fallbacks that cover the brief window between page load and this
// plugin's setup running.

import { defineThemePlugin } from "../themes/defineThemePlugin";

export default defineThemePlugin({
  id: "dark",
  label: "Dark",
  scheme: "dark",
  order: 0,
  palette: {
    // ---------- Brand — Lyra signature green ----------
    "color-accent":         "#1ed760",
    "color-accent-border":  "#1db954",
    "color-accent-press":   "#169c46",

    // ---------- Surface ladder ----------
    // Canvas + surface (L1) are manual anchors; L2-L4 derive via
    // color-mix() in tokens.css from --color-text + --depth-step.
    "color-bg":             "#010102",
    "color-surface":        "#181a1d",

    // ---------- Ink ----------
    // Ink ladder calibrated so the small-text tiers stay above WCAG AA
    // (4.5:1 on dark canvas). text-faint was #62666d — that read ≈3.6:1
    // on canvas, which fails AA for body / caption sizes (11-12px). New
    // #76787e reads ≈4.7:1 on canvas while staying clearly subordinate
    // to text-muted. Same hierarchy intent, accessible to low-vision
    // users and on glossy displays.
    "color-text":           "#f7f8f8",
    "color-text-bright":    "#ffffff",
    "color-text-soft":      "#d0d6e0",
    "color-text-muted":     "#9ea3ac", // bumped from #8a8f98 → ~5.6:1 on canvas
    "color-text-faint":     "#76787e", // bumped from #62666d → ~4.7:1 on canvas
    "color-text-on-accent": "#000000", // black ink reads on bright green

    // ---------- Hairlines ----------
    "color-border":         "#23252a",
    "color-border-soft":    "#34343a",
    "color-divider":        "#3e3e44",
    "color-app-divider":    "#23252a",

    // ---------- Semantic ----------
    "color-negative":       "#ee0000",
    "color-warning":        "#f5a623",
    "color-info":           "#0070f3",
    "color-success":        "#27a644",
  },
});
