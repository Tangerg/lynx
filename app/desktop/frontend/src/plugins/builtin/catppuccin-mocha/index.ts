// Built-in plugin: Catppuccin Mocha theme.
//
// Canonical palette from catppuccin/catppuccin (Mocha — the popular dark
// variant). Default accent is mauve (#cba6f7), matching the official
// catppuccin VS Code extension's accent pick.

import { definePlugin } from "@/plugins/sdk";

const tokens: Record<string, string> = {
  // ---------- Brand — mauve (Catppuccin's signature accent) ----------
  "color-accent":         "#cba6f7",
  "color-accent-border":  "#b18cf3",
  "color-accent-press":   "#9572ed",

  // ---------- Surface ladder ----------
  // mantle (#181825) under base (#1e1e2e) — canvas darker than surface.
  "color-bg":             "#181825",
  "color-surface":        "#1e1e2e",
  "depth-step":           "5%",

  // ---------- Ink — text → subtext1 → subtext0 → overlay1 ladder ----------
  "color-text":           "#cdd6f4",
  "color-text-bright":    "#ffffff",
  "color-text-soft":      "#bac2de", // subtext1
  "color-text-muted":     "#a6adc8", // subtext0
  "color-text-faint":     "#7f849c", // overlay1
  "color-text-on-accent": "#1e1e2e", // dark text on the soft mauve

  // ---------- Hairlines — surface0/1/2 ladder ----------
  "color-border":         "#45475a", // surface1
  "color-border-soft":    "#585b70", // surface2
  "color-divider":        "#6c7086", // overlay0
  "color-app-divider":    "#45475a",

  // ---------- Semantic ----------
  "color-negative":       "#f38ba8", // red
  "color-warning":        "#fab387", // peach
  "color-info":           "#89b4fa", // blue
  "color-success":        "#a6e3a1", // green

  // ---------- CTA ----------
  "color-cta":            "var(--color-accent)",
  "color-cta-hover":      "var(--color-accent-border)",
  "color-cta-text":       "var(--color-text-on-accent)",

  // ---------- Shadows (dark policy) ----------
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
  name: "lyra.builtin.theme-catppuccin-mocha",
  version: "1.0.0",
  setup({ host }) {
    host.theme.registerTheme({
      id: "catppuccin-mocha",
      label: "Catppuccin Mocha",
      scheme: "dark",
      icon: "moon",
      order: 40,
      tokens,
    });
  },
});
