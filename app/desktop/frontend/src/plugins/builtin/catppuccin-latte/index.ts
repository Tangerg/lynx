// Built-in plugin: Catppuccin Latte theme.
//
// The light counterpart to Mocha. Catppuccin Latte's mauve is far more
// saturated than Mocha's (#8839ef vs #cba6f7) — it has to bite against
// the bright surface to read as the accent.

import { definePlugin } from "@/plugins/sdk";

const tokens: Record<string, string> = {
  // ---------- Brand — Latte mauve (more saturated than Mocha's) ----------
  "color-accent":         "#8839ef",
  "color-accent-border":  "#6f25d4",
  "color-accent-press":   "#5817b3",

  // ---------- Surface ladder ----------
  // mantle (#e6e9ef) is canvas, base (#eff1f5) is editor surface.
  "color-bg":             "#e6e9ef",
  "color-surface":        "#eff1f5",
  "depth-step":           "5%",

  // ---------- Ink ----------
  "color-text":           "#4c4f69",
  "color-text-bright":    "#000000",
  "color-text-soft":      "#5c5f77", // subtext1
  "color-text-muted":     "#6c6f85", // subtext0
  "color-text-faint":     "#8c8fa1", // overlay1
  "color-text-on-accent": "#ffffff", // white reads on the deep mauve

  // ---------- Hairlines — surface0/1/2 ----------
  "color-border":         "#ccd0da", // surface0
  "color-border-soft":    "#bcc0cc", // surface1
  "color-divider":        "#acb0be", // surface2
  "color-app-divider":    "#ccd0da",

  // ---------- Semantic ----------
  "color-negative":       "#d20f39", // red
  "color-warning":        "#fe640b", // peach
  "color-info":           "#1e66f5", // blue
  "color-success":        "#40a02b", // green

  // ---------- CTA ----------
  "color-cta":            "var(--color-accent)",
  "color-cta-hover":      "var(--color-accent-border)",
  "color-cta-text":       "var(--color-text-on-accent)",

  // ---------- Shadows (light policy) ----------
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
  name: "lyra.builtin.theme-catppuccin-latte",
  version: "1.0.0",
  setup({ host }) {
    host.theme.registerTheme({
      id: "catppuccin-latte",
      label: "Catppuccin Latte",
      scheme: "light",
      icon: "sun",
      order: 41,
      tokens,
    });
  },
});
