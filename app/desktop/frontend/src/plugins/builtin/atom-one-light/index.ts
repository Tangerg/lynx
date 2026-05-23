// Built-in plugin: Atom One Light theme.
//
// Canonical palette from the `one-light-syntax` Atom package and the VS Code
// `Atom One Light` theme — identical values across the two. Surfaces use
// Atom's editor #fafafa + sidebar #f0f0f1; ink ladder runs from the
// foreground #383a42 down toward the comment shade #a0a1a7. The accent is
// Atom's cursor blue #526fff (a hair more saturated than the Dark variant's
// #528bff), which doubles as the activity-bar marker on the VS Code side.
//
// Mirrors the structure of atom-one-dark/index.ts — see that file for
// commentary on the "theme as plugin" pattern.

import { definePlugin } from "@/plugins/sdk";

const tokens: Record<string, string> = {
  // ---------- Brand / cursor — Atom's One Light blue ----------
  "color-accent":         "#526fff",
  "color-accent-border":  "#4060e8",
  "color-accent-press":   "#2d4ac6",

  // ---------- Surface ladder ----------
  // canvas: One Light's sidebar/panel background (#f0f0f1) — a notch
  // darker than the editor #fafafa, so the panel-on-canvas hierarchy
  // reads the right direction in light mode too.
  "color-bg":             "#f0f0f1",
  "color-surface":        "#fafafa",
  "depth-step":           "5%",

  // ---------- Ink ----------
  "color-text":           "#383a42", // Atom foreground
  "color-text-bright":    "#000000",
  "color-text-soft":      "#383a42",
  "color-text-muted":     "#696c77",
  "color-text-faint":     "#a0a1a7", // Atom comment shade
  "color-text-on-accent": "#ffffff", // white on #526fff blue

  // ---------- Hairlines ----------
  // Atom One Light's selection band is #e5e5e6 — that becomes the
  // default border. Soft / divider step up from there.
  "color-border":         "#e5e5e6",
  "color-border-soft":    "#d4d4d6",
  "color-divider":        "#a0a1a7",
  "color-app-divider":    "#e5e5e6",

  // ---------- Semantic — straight from One Light syntax ----------
  "color-negative":       "#e45649", // Atom red — errors
  "color-warning":        "#986801", // Atom orange — constants
  "color-info":           "#4078f2", // Atom blue — functions/links
  "color-success":        "#50a14f", // Atom green — strings/run-clean

  // ---------- CTA ----------
  // Unlike the Vercel-style default light (black-on-white CTA), the
  // canonical Atom feel uses the cursor blue for the primary action —
  // matches One Light's accent-button color across the IDE.
  "color-cta":            "var(--color-accent)",
  "color-cta-hover":      "var(--color-accent-border)",
  "color-cta-text":       "var(--color-text-on-accent)",

  // ---------- Shadows ----------
  // Light surfaces need real shadows for depth (unlike dark, where
  // surface ladder + hairlines do the work). Same stacked-shadow
  // ladder our default light theme uses.
  "shadow-xs":            "0 1px 2px rgba(15, 15, 15, 0.04)",
  "shadow-sm":
    "0 1px 2px rgba(15, 15, 15, 0.04), " +
    "0 2px 6px rgba(15, 15, 15, 0.06)",
  "shadow-md":
    "0 2px 4px rgba(15, 15, 15, 0.04), " +
    "0 8px 20px rgba(15, 15, 15, 0.10)",
  "shadow-lg":
    "0 4px 12px rgba(15, 15, 15, 0.08), " +
    "0 24px 60px -12px rgba(15, 15, 15, 0.18)",
  "shadow-card":          "var(--shadow-sm)",
  "shadow-dialog":        "var(--shadow-lg)",
  "shadow-pop":           "var(--shadow-lg)",
  "shadow-soft":          "var(--shadow-xs)",
  "shadow-glow":          "0 0 12px color-mix(in srgb, var(--color-accent) 40%, transparent)",
  "shadow-input-focus":
    "0 0 0 3px color-mix(in srgb, var(--color-accent) 14%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};

export default definePlugin({
  name: "lyra.builtin.theme-atom-one-light",
  version: "1.0.0",
  setup({ host }) {
    host.theme.registerTheme({
      id: "atom-one-light",
      label: "Atom One Light",
      scheme: "light",
      icon: "sun",
      order: 11,
      tokens,
    });
  },
});
