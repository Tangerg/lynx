// Lyra Light — Vercel-inspired bright canvas + white surface. Pure
// black-on-white CTA so the green accent stays reserved for live state.

import { defineThemePlugin } from "./kit/defineThemePlugin";

const c = {
  green: "#15883e",

  canvas: "#fafafa",
  surface1: "#ffffff",

  // Calibrated to clear WCAG AA on 11-12px text against canvas.
  inkBright: "#000000",
  ink: "#171717",
  inkSoft: "#4d4d4d",
  inkMuted: "#5e5e5e",
  inkFaint: "#707070",

  hairline: "#ebebeb",
  hairStrong: "#d4d4d6",
  hairTertiary: "#a1a1a1",
};

export default defineThemePlugin({
  id: "light",
  label: "Light",
  scheme: "light",
  order: 1,

  brand: {
    accent: c.green,
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
    appDivider: c.hairline,
  },
  semantic: {
    negative: "#ee0000",
    warning: "#f5a623",
    info: "#0070f3",
    success: "#15883e",
  },
  cta: {
    cta: "#000000",
    ctaHover: "#222222",
    ctaText: "#ffffff",
  },
});
