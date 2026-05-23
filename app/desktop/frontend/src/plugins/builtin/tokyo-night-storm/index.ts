// Built-in plugin: Tokyo Night Storm theme.
//
// Canonical palette from enkia/tokyonight (Storm variant) — the same
// palette VS Code's `Tokyo Night` extension and the Vim/Neovim port use.
// Storm is the slightly-lighter sibling of the original Tokyo Night Night
// variant; we pick Storm because the higher surface tone gives the
// cards-on-canvas layout more headroom.

import { definePlugin } from "@/plugins/sdk";

const tokens: Record<string, string> = {
  // ---------- Brand — Tokyo Night's signature blue ----------
  "color-accent":         "#7aa2f7",
  "color-accent-border":  "#5d86e6",
  "color-accent-press":   "#4068c5",

  // ---------- Surface ladder ----------
  // bg_dark (#1f2335) sits under bg (#24283b) — natural canvas / surface pair.
  "color-bg":             "#1f2335",
  "color-surface":        "#24283b",
  "depth-step":           "5%",

  // ---------- Ink ----------
  "color-text":           "#c0caf5", // fg
  "color-text-bright":    "#ffffff",
  "color-text-soft":      "#a9b1d6", // fg_dark
  "color-text-muted":     "#787c99",
  "color-text-faint":     "#565f89", // comment
  "color-text-on-accent": "#1a1b26", // dark ink reads on the bright blue

  // ---------- Hairlines ----------
  "color-border":         "#3b4261", // fg_gutter
  "color-border-soft":    "#545c7e",
  "color-divider":        "#565f89",
  "color-app-divider":    "#3b4261",

  // ---------- Semantic — verbatim from Tokyo Night syntax ----------
  "color-negative":       "#f7768e", // red
  "color-warning":        "#ff9e64", // orange
  "color-info":           "#7dcfff", // cyan
  "color-success":        "#9ece6a", // green

  // ---------- CTA ----------
  "color-cta":            "var(--color-accent)",
  "color-cta-hover":      "var(--color-accent-border)",
  "color-cta-text":       "var(--color-text-on-accent)",

  // ---------- Shadows (dark policy — surface ladder does the work) ----------
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
  name: "lyra.builtin.theme-tokyo-night-storm",
  version: "1.0.0",
  setup({ host }) {
    host.theme.registerTheme({
      id: "tokyo-night-storm",
      label: "Tokyo Night Storm",
      scheme: "dark",
      icon: "moon",
      order: 20,
      tokens,
    });
  },
});
