// Premium Dark — deep, refined near-black with a subtle cool undertone.
// Not pure #000; the slight chroma gives depth without reading as flat gray.
// oklch surfaces + alpha hairlines for a frosted, atmospheric feel.
// Glossy accent glow (--shadow-accent-glow) adds a luminous send-button pop.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // Accent — luminous, slightly desaturated indigo that glows softly
  // against the deep dark canvas. Used sparingly (restraint = premium).
  accent: "#7b8efa",
  textOnAccent: "#030408",

  // Deep near-black with a subtle cool undertone (oklch).
  canvas: "oklch(0.12 0.008 265)",
  surface1: "oklch(0.165 0.012 265)",

  // Ink — slightly warmer and more restrained than the lyra-dark grays,
  // so prose reads as refined, not clinical.
  inkBright: "#ffffff",
  ink: "#e8eaed",
  inkSoft: "#b8bcc4",
  inkMuted: "#8a8f98",
  inkFaint: "#5c6068",

  // Hairlines — alpha-based so they sit softly on glass surfaces
  // and don't fight the atmospheric depth.
  hairline: "rgba(255, 255, 255, 0.08)",
  hairStrong: "rgba(255, 255, 255, 0.05)",
  hairTertiary: "rgba(255, 255, 255, 0.06)",
};

export default defineThemePlugin({
  id: "premium-dark",
  label: "Premium Dark",
  scheme: "dark",
  order: 2,

  brand: {
    accent: c.accent,
    textOnAccent: c.textOnAccent,
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
    appDivider: c.hairTertiary,
  },
  semantic: {
    negative: "#f85149",
    warning: "#f0a936",
    info: "#58a6ff",
    success: "#3fb950",
  },
  extras: {
    "shadow-accent-glow":
      "0 0 20px color-mix(in oklab, var(--color-accent) 35%, transparent), 0 0 40px color-mix(in oklab, var(--color-accent) 15%, transparent)",
  },
});
