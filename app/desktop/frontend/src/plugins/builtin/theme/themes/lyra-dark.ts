// Lyra Dark — system default. Synthesis of Linear (surface ladder) +
// Vercel (typography + elevation). Source of truth; tokens.css `:root`
// only stands in until this plugin's setup runs.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // Brand — Lyra signature green. accentBorder/Press derived by
  // defineThemePlugin via colord.darken().
  green: "#1ed760",

  // Surface anchors
  canvas: "#010102", // page background — faint blue tint, not pure black
  surface1: "#181a1d", // panel / sidebar / message bubble

  // Ink
  inkBright: "#ffffff",
  ink: "#f7f8f8",
  inkSoft: "#d0d6e0",
  // Calibrated for WCAG AA at 11-12px sizes on canvas (~5.6:1 / ~4.7:1).
  inkMuted: "#9ea3ac",
  inkFaint: "#76787e",

  // Hairlines — literal hex (DESIGN.md §2: precise > alpha-blended)
  hairline: "#23252a",
  hairStrong: "#34343a",
  hairTertiary: "#3e3e44",
};

export default defineThemePlugin({
  id: "dark",
  label: "Dark",
  scheme: "dark",
  order: 0,

  brand: {
    accent: c.green,
    textOnAccent: "#000000", // black ink reads on bright green
  },
  surfaces: {
    bg: c.canvas,
    surface: c.surface1,
  },
  ink: {
    text: c.ink,
    textBright: c.inkBright,
    textSoft: c.inkSoft,
    textMuted: c.inkMuted,
    textFaint: c.inkFaint,
  },
  borders: {
    border: c.hairline,
    borderSoft: c.hairStrong,
    divider: c.hairTertiary,
    appDivider: c.hairline,
  },
  semantic: {
    // Dark-tuned: the raw web-saturated values (#ee0000 / #0070f3) vibrate and
    // bleed at the edges on the near-black canvas. Desaturated + lifted in
    // luminance per Apple Dark Mode / Ant dark guidance — GitHub-dark-aligned.
    negative: "#f85149",
    warning: "#f0a936",
    info: "#58a6ff",
    success: "#3fb950",
  },
});
