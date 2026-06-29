// Lyra Light — clean white main area + subtle gray chrome (flush layout).
// Pure black-on-white CTA, so the blue accent stays reserved for live state.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // Restrained blue accent (near-monochrome direction); the CTA stays black.
  accent: "#2563eb",

  // Flush layout: canvas = clean white main area, surface = subtle gray chrome.
  canvas: "#ffffff",
  surface1: "#f5f5f7",

  // Calibrated to clear WCAG AA on 11-12px text against canvas.
  inkBright: "#000000",
  ink: "#0d0d0d",
  inkSoft: "#5d5d5d",
  inkMuted: "#6e6e80",
  inkFaint: "#9ca3af",

  hairline: "#e5e5e5",
  hairStrong: "#f0f0f0",
  hairTertiary: "#e5e5e5",
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
