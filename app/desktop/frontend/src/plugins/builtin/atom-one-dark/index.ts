// Built-in plugin: Atom One Dark theme.
//
// Canonical palette from the `one-dark-syntax` Atom package and the VS Code
// `One Dark Pro` extension — the two ship identical values. Surfaces use
// Atom's editor + activity-bar pair; ink ladder is foreground (#abb2bf)
// stepped down toward the comment shade (#5c6370). The accent is Atom's
// cursor blue (#528bff), which doubles as the active activity-bar marker
// in One Dark Pro.
//
// Adding a theme = drop a file like this in plugins/builtin/, register
// once, list in builtin/index.ts. The theme picker (Settings → Appearance)
// reads from the plugin registry, so the new entry shows up automatically.

import { definePlugin } from "@/plugins/sdk";

const tokens: Record<string, string> = {
  // ---------- Brand / cursor — Atom's signature blue ----------
  "color-accent":         "#528bff",
  "color-accent-border":  "#4078e6",
  "color-accent-press":   "#2f63cc",

  // ---------- Surface ladder ----------
  // canvas (outer frame): one step darker than the editor bg, matching
  // Atom's panel.background. Surface is the editor bg #282c34. L2-L4
  // derive via the color-mix() formula in tokens.css against the new
  // (light-blue-grey) --color-text, so they read as "lifted" naturally.
  "color-bg":             "#1c2026",
  "color-surface":        "#282c34",
  "depth-step":           "5%",

  // ---------- Ink ----------
  "color-text":           "#abb2bf",
  "color-text-bright":    "#ffffff",
  "color-text-soft":      "#abb2bf",
  "color-text-muted":     "#828997",
  "color-text-faint":     "#5c6370", // Atom's comment shade
  "color-text-on-accent": "#ffffff", // white reads on the #528bff blue

  // ---------- Hairlines ----------
  // Atom uses #3e4451 for selection, which doubles as the canonical
  // divider tone. Steps down to #4b5263 for emphasized dividers.
  "color-border":         "#3a3f4b",
  "color-border-soft":    "#4b5263",
  "color-divider":        "#5c6370",
  "color-app-divider":    "#3a3f4b",

  // ---------- Semantic — straight from One Dark syntax ----------
  "color-negative":       "#e06c75", // Atom red — errors
  "color-warning":        "#d19a66", // Atom orange — constants
  "color-info":           "#61afef", // Atom blue — functions/links
  "color-success":        "#98c379", // Atom green — strings/run-clean

  // ---------- CTA — accent-driven (dark theme convention) ----------
  "color-cta":            "var(--color-accent)",
  "color-cta-hover":      "var(--color-accent-border)",
  "color-cta-text":       "var(--color-text-on-accent)",

  // ---------- Shadows ----------
  // Match the built-in dark policy: inner cards don't shadow (depth via
  // surface ladder), only the floating-overlay layer gets a real one.
  "shadow-xs":            "none",
  "shadow-sm":            "none",
  "shadow-md":            "none",
  "shadow-lg":
    "inset 0 1px 0 rgba(255, 255, 255, 0.04), " +
    "0 1px 2px rgba(0, 0, 0, 0.40), " +
    "0 8px 16px -4px rgba(0, 0, 0, 0.50), " +
    "0 24px 32px -8px rgba(0, 0, 0, 0.60), " +
    "inset 0 0 0 1px var(--color-border)",
  "shadow-card":          "none",
  "shadow-dialog":        "var(--shadow-lg)",
  "shadow-soft":          "none",
  "shadow-pop":           "var(--shadow-lg)",
  "shadow-glow":          "0 0 12px color-mix(in srgb, var(--color-accent) 50%, transparent)",
  "shadow-input-focus":
    "0 0 0 2px color-mix(in srgb, var(--color-accent) 30%, transparent), " +
    "inset 0 0 0 1px var(--color-border-soft)",
};

export default definePlugin({
  name: "lyra.builtin.theme-atom-one-dark",
  version: "1.0.0",
  setup({ host }) {
    host.theme.registerTheme({
      id: "atom-one-dark",
      label: "Atom One Dark",
      scheme: "dark",
      icon: "moon",
      order: 10,
      tokens,
    });
  },
});
