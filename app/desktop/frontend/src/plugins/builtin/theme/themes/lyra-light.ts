// Lyra Light — clean white main area + subtle gray chrome (flush layout).
// Pure black-on-white CTA, so the blue accent stays reserved for live state.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // Geist blue-700 accent (Vercel design.md).
  accent: "#006bff",

  // Cool-slate surface ladder — pristine white editor canvas, then a SINGLE
  // hue (262°, the accent's OKLCH family) at a constant low chroma for the
  // chrome, stepping darker only in lightness (JetBrains New UI model: editor
  // lightest, chrome + raised states a touch darker + cool). Region + card
  // separation is this L delta, no lines, no shadows. Chroma is tiny (0.007)
  // so on near-white it reads as a cool grey, never a tint.
  canvas: "#ffffff",
  surface1: "oklch(0.967 0.007 262)",
  surface2: "oklch(0.933 0.007 262)",
  surface3: "oklch(0.899 0.007 262)",
  surface4: "oklch(0.865 0.007 262)",

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
    surface2: c.surface2,
    surface3: c.surface3,
    surface4: c.surface4,
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
