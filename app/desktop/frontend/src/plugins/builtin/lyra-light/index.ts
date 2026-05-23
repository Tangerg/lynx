// Built-in plugin: Lyra Light — Vercel dashboard inspired.
//
// Bright canvas + white surface. CTAs go pure black-on-white (Vercel
// signature), decoupled from the accent — so accent can stay reserved
// for "live state" indicators without forcing primary buttons green.

import { defineThemePlugin } from "../themes/defineThemePlugin";

export default defineThemePlugin({
  id: "light",
  label: "Light",
  scheme: "light",
  order: 1,
  palette: {
    // ---------- Brand — Lyra green, dimmed for white background ----------
    "color-accent":         "#15883e",
    "color-accent-border":  "#117134",
    "color-accent-press":   "#0c5d2a",

    // ---------- Surface ladder ----------
    "color-bg":             "#fafafa",
    "color-surface":        "#ffffff",

    // ---------- Ink — Vercel #171717 / #4d4d4d / #6f6f6f ladder ----------
    "color-text":           "#171717",
    "color-text-bright":    "#000000",
    "color-text-soft":      "#4d4d4d",
    "color-text-muted":     "#6f6f6f",
    "color-text-faint":     "#a1a1a1",
    "color-text-on-accent": "#ffffff",

    // ---------- Hairlines — Vercel #ebebeb / #d4d4d6 ladder ----------
    "color-border":         "#ebebeb",
    "color-border-soft":    "#d4d4d6",
    "color-divider":        "#a1a1a1",
    "color-app-divider":    "#ebebeb",

    // ---------- Semantic ----------
    "color-negative":       "#ee0000",
    "color-warning":        "#f5a623",
    "color-info":           "#0070f3",
    "color-success":        "#15883e",
  },
  // Override CTA — Vercel's signature black-on-white instead of the
  // accent-driven default. Accent stays reserved for live indicators.
  cta: {
    "color-cta":       "#000000",
    "color-cta-hover": "#222222",
    "color-cta-text":  "#ffffff",
  },
});
