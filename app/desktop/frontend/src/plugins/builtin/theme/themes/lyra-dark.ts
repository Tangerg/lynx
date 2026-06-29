// Lyra Dark — system default. Synthesis of Linear (surface ladder) +
// Vercel (typography + elevation). Source of truth; tokens.css `:root`
// only stands in until this plugin's setup runs.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // Accent — luminous indigo, used sparingly for live state only.
  accent: "#7b8efa",

  // Surface anchors (flush layout: canvas = main reading area, surface = chrome)
  canvas: "oklch(0.12 0.012 260)", // cool slate near-black
  surface1: "oklch(0.205 0.014 260)", // meaningful lift from canvas

  // Ink
  inkBright: "#ffffff",
  ink: "#ececec",
  inkSoft: "#b8bcc4",
  inkMuted: "#8a8f98",
  inkFaint: "#5c6068",

  // Hairlines — alpha-based so they sit softly on dark surfaces
  hairline: "rgba(255, 255, 255, 0.10)",
  hairStrong: "rgba(255, 255, 255, 0.06)",
  hairTertiary: "rgba(255, 255, 255, 0.08)",
};

export default defineThemePlugin({
  id: "dark",
  label: "Dark",
  scheme: "dark",
  order: 0,

  brand: {
    accent: c.accent,
    textOnAccent: "#030408", // near-black ink on the indigo accent
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
    negative: "#f85149",
    warning: "#f0a936",
    info: "#58a6ff",
    success: "#3fb950",
  },
});
