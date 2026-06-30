// Lyra Light — clean white main area + subtle gray chrome (flush layout).
// Pure black-on-white CTA, so the blue accent stays reserved for live state.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // Geist blue-700 accent (Vercel design.md).
  accent: "#006bff",

  // Geist background scale: bg-100 white, bg-200 subtle separation.
  canvas: "#ffffff",
  surface1: "#f5f5f3",

  // Geist gray scale (text/icons): gray-1000 / 900 / 700 / 600.
  inkBright: "#000000",
  ink: "#171717",
  inkSoft: "#4d4d4d",
  inkMuted: "#8f8f8f",
  inkFaint: "#a0a0a0",

  // Geist gray-alpha scale (translucent borders/dividers).
  hairline: "rgb(17 17 17 / 0.08)",
  hairStrong: "rgb(17 17 17 / 0.16)",
  hairTertiary: "rgb(17 17 17 / 0.05)",
};

export default defineThemePlugin({
  id: "light",
  label: "Light",
  scheme: "light",
  order: 1,

  brand: {
    accent: c.accent,
    textOnAccent: "#ffffff",
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
    negative: "#ea001d", // red-800
    warning: "#ffa600", // amber-600
    info: "#006bff", // blue-700
    success: "#28a948", // green-700
  },
  cta: {
    cta: "#171717", // gray-1000 solid fill
    ctaHover: "#4d4d4d", // gray-900
    ctaText: "#ffffff", // background-100
  },
});
