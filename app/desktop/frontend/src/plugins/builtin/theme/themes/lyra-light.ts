// Lyra Light — Agent Studio skin: white canvas, neutral gray chrome, pink CTA.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  accent: "#d92662",

  // JetBrains-style region split: canvas is white; durable chrome is a neutral
  // gray ladder. The ladder is deliberately achromatic so the page stops reading
  // as foggy or tinted while still keeping large areas distinct.
  canvas: "#ffffff",
  surface1: "#f2f2f3",
  surface2: "#e8e8e9",
  surface3: "#dededf",
  surface4: "#d2d2d4",

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
    cta: "#d92662",
    ctaHover: "#c61f56",
    ctaText: "#ffffff",
  },
});
