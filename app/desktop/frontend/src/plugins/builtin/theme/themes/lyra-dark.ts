// Lyra Dark — system default. Synthesis of Linear (surface ladder) +
// Vercel (typography + elevation). Source of truth; tokens.css `:root`
// only stands in until this plugin's setup runs.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // Geist blue-700 accent (same hue as light; reads clean on dark).
  accent: "#006bff",

  // Geist-dark surfaces: near-black canvas, gray-1000 lifted chrome.
  canvas: "#0a0a0a",
  surface1: "#171717",

  // Ink
  inkBright: "#ffffff",
  ink: "#ededed",
  inkSoft: "#a1a1a1",
  inkMuted: "#8f8f8f",
  inkFaint: "#636363",

  // Hairlines — alpha-based so they sit softly on dark surfaces
  hairline: "rgba(255, 255, 255, 0.10)",
  hairStrong: "rgba(255, 255, 255, 0.21)",
  hairTertiary: "rgba(255, 255, 255, 0.08)",
};

export default defineThemePlugin({
  id: "dark",
  label: "Dark",
  scheme: "dark",
  order: 0,

  brand: {
    accent: c.accent,
    textOnAccent: "#ffffff", // white ink on the blue accent
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
  },
  semantic: {
    negative: "#fc0035", // red-700
    warning: "#ffc543", // amber-500
    info: "#006bff", // blue-700
    success: "#4ce15e", // green-600
  },
});
