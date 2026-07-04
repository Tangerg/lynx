// Lyra Dark — system default. Synthesis of Linear (surface ladder) +
// Vercel (typography). Source of truth; tokens.css `:root` only stands in
// until this plugin's setup runs.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // Geist blue-700 accent (same hue as light; reads clean on dark).
  accent: "#006bff",

  // Cool-slate surface ladder — a SINGLE hue (262°, the accent's OKLCH family)
  // at a constant low chroma, stepped only in lightness (JetBrains New UI
  // model). This replaces the old dead-pure-neutral greys: a whisper of shared
  // cool cast is what makes a grey read as "designed" rather than flat. Region
  // + card separation is carried entirely by this even L delta — no lines, no
  // shadows. The chroma is deliberately low (0.02) to stay a near-neutral slate,
  // not a blue theme.
  canvas: "oklch(0.145 0.02 262)",
  surface1: "oklch(0.205 0.02 262)",
  surface2: "oklch(0.245 0.02 262)",
  surface3: "oklch(0.285 0.02 262)",
  surface4: "oklch(0.325 0.02 262)",

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
    negative: "#fc0035", // red-700
    warning: "#ffc543", // amber-500
    info: "#006bff", // blue-700
    success: "#4ce15e", // green-600
  },
  // Primary CTA — inverting ink button (near-white fill on dark), mirroring the
  // light scheme's ink-on-white. Accent (blue) stays reserved for "live".
  cta: {
    cta: "#ededed",
    ctaHover: "#ffffff",
    ctaText: "#0a0a0a",
  },
});
