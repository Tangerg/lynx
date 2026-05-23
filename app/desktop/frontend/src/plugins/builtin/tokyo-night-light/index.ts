// Built-in plugin: Tokyo Night Light theme.
//
// Canonical palette from enkia/tokyonight (Day / Light variant). Tokyo
// Night Light is intentionally muted/earthy compared to other light
// themes — designed as a low-eye-strain sibling to the Storm dark.

import { definePlugin } from "@/plugins/sdk";

const tokens: Record<string, string> = {
  // ---------- Brand — Tokyo Night Day's darker blue ----------
  "color-accent":         "#34548a",
  "color-accent-border":  "#2a4373",
  "color-accent-press":   "#1f335a",

  // ---------- Surface ladder ----------
  // bg_dark (#cbccd1) sits under bg (#d5d6db) — canvas / surface.
  "color-bg":             "#cbccd1",
  "color-surface":        "#d5d6db",
  "depth-step":           "5%",

  // ---------- Ink ----------
  "color-text":           "#343b58", // fg
  "color-text-bright":    "#1a1b26",
  "color-text-soft":      "#343b58",
  "color-text-muted":     "#4c505e", // fg_dark
  "color-text-faint":     "#848cb5", // comment
  "color-text-on-accent": "#ffffff", // white on the darker blue

  // ---------- Hairlines ----------
  "color-border":         "#a8aecb", // fg_gutter
  "color-border-soft":    "#c4c8d8",
  "color-divider":        "#848cb5",
  "color-app-divider":    "#a8aecb",

  // ---------- Semantic — Tokyo Night Day syntax ----------
  "color-negative":       "#8c4351", // red
  "color-warning":        "#965027", // orange
  "color-info":           "#0f4b6e", // cyan
  "color-success":        "#485e30", // green

  // ---------- CTA ----------
  "color-cta":            "var(--color-accent)",
  "color-cta-hover":      "var(--color-accent-border)",
  "color-cta-text":       "var(--color-text-on-accent)",

  // ---------- Shadows (light policy — real shadow ladder for depth) ----------
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
  name: "lyra.builtin.theme-tokyo-night-light",
  version: "1.0.0",
  setup({ host }) {
    host.theme.registerTheme({
      id: "tokyo-night-light",
      label: "Tokyo Night Light",
      scheme: "light",
      icon: "sun",
      order: 21,
      tokens,
    });
  },
});
