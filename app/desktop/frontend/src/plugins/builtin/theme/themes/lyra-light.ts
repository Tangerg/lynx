// Lyra Light — clean white main area + subtle gray chrome (flush layout).
// Pure black-on-white CTA, so the blue accent stays reserved for live state.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // Geist blue-700 accent (Vercel design.md).
  accent: "#006bff",

  // Geist background scale: bg-100 white, bg-200 subtle separation.
  canvas: "#ffffff",
  surface1: "#fafafa",

  // Geist gray scale (text/icons): gray-1000 / 900 / 700 / 600.
  inkBright: "#000000",
  ink: "#171717",
  inkSoft: "#4d4d4d",
  inkMuted: "#8f8f8f",
  inkFaint: "#a8a8a8",

  // Geist gray-alpha scale (translucent borders/dividers).
  hairline: "#00000014", // gray-alpha-400 — default border
  hairStrong: "#00000036", // gray-alpha-500 — focus/emphasized
  hairTertiary: "#0000001a", // gray-alpha-300 — divider
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
