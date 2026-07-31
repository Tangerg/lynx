// Lyra Dark — system default.
//
// Mirror of the light scheme's separation model: card and shell sit at nearly the
// same value and the seam ring plus depth shadow do the dividing. On dark that
// matters more, not less — a lightness step big enough to read as a region split
// makes the chrome look like a lighter panel glued on, while a near-black pair
// with one hairline between them reads as depth.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // Same accent hue as light; reads clean on near-black.
  accent: "#006bff",

  // Card is a whisper above the shell — enough for the frosted drawer (card color
  // at 72%) to separate, not enough to read as a different material.
  canvas: "#101010",
  surface1: "#0e0e0e",

  // surface2/3/4 derive as ink mixes off `surface1`, same as light.

  // Ink
  inkBright: "#ffffff",
  ink: "#f5f5f5",
  inkSoft: "#a1a1a1",
  inkMuted: "#818181",
  // The quietest readable text rung still clears 4.5:1 on both canvas and
  // surface. Hierarchy comes from size/weight/placement, not illegible ink.
  inkFaint: "#7c7c7c",

  // Hairlines — very low alpha. The seam ring derives from `border`, and on a
  // near-black surface even 10% white reads as a drawn line rather than an edge.
  hairline: "rgb(255 255 255 / 0.04)",
  hairStrong: "rgb(255 255 255 / 0.1)",
  hairTertiary: "rgb(255 255 255 / 0.03)",
};

export default defineThemePlugin({
  id: "dark",
  label: "Dark",
  scheme: "dark",
  order: 0,

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
    negative: "#fc0035", // red-700
    warning: "#ffc543", // amber-500
    info: "#006bff", // blue-700
    success: "#4ce15e", // green-600
  },
  // Primary CTA — inverting ink button (near-white fill on dark), mirroring the
  // light scheme's ink-on-white. Accent (blue) stays reserved for "live".
  cta: {
    cta: "#f5f5f5",
    ctaHover: "#ffffff",
    ctaText: "#171717",
  },
});
