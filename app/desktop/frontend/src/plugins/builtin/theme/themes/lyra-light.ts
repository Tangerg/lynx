// Lyra Light — the default skin. Vercel Geist design language: white canvas,
// neutral gray chrome separated by background delta (no rules), Geist ink ramp.
// Accent (blue-700) is reserved for live / focus / links; the primary CTA is the
// inverting ink-on-white button (Vercel / ChatGPT signature), so blue stays rare.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // Geist blue-700 — the one accent. Live indicators, focus rings, links.
  accent: "#006bff",

  // JetBrains-style region split: canvas is white; durable chrome (sidebar,
  // dock, cards) is a neutral achromatic gray ladder. Large regions read
  // distinct by brightness delta, never by a grey rule.
  canvas: "#ffffff", // background-100
  surface1: "#f2f2f2", // gray-100 — sidebar / dock / card chrome
  surface2: "#e8e8e8", // hover row, user bubble, chip
  surface3: "#dedede", // active row, dropdown
  surface4: "#d1d1d1", // deepest lifted

  // Geist gray scale (text/icons): gray-1000 / 900 / 700 / 600.
  inkBright: "#000000",
  ink: "#171717", // gray-1000
  inkSoft: "#4d4d4d", // gray-900 — body / secondary
  inkMuted: "#8f8f8f", // gray-700 — meta / inactive
  inkFaint: "#a8a8a8", // gray-600 — footnote / disabled

  // Geist gray-alpha scale (translucent borders/dividers/hover fills).
  hairline: "rgb(17 17 17 / 0.08)",
  hairStrong: "rgb(17 17 17 / 0.14)",
  hairTertiary: "rgb(17 17 17 / 0.05)",
};

export default defineThemePlugin({
  id: "light",
  label: "Light",
  scheme: "light",
  order: 0,

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
  // Primary CTA — inverting ink-on-white (Vercel primary button). Hover goes
  // pure black (Geist `.press:hover`). Accent stays reserved for "live".
  cta: {
    cta: "#171717",
    ctaHover: "#000000",
    ctaText: "#ffffff",
  },
});
