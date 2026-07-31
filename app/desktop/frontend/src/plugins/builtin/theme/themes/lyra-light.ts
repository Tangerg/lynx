// Lyra Light — the default skin.
//
// The separation model is a raised content card over a frosted drawer, so the
// two big regions sit at almost the same value: the seam ring and the card's
// depth shadow carry the split, not a brightness delta. That is why `surface`
// (the shell the drawer floats on) is a hair BELOW pure white instead of a step
// of grey — a grey rail beside a white column reads as two pasted rectangles.
//
// Accent (blue-700) is reserved for live / focus / links; the primary CTA is the
// inverting ink-on-white button, so blue stays rare.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  // The one accent. Live indicators, focus rings, links.
  accent: "#006bff",

  // Card is pure white; the shell behind the drawer is a half-step down. The
  // drawer itself is the card color at partial opacity over this shell (see
  // `--app-drawer-surface`), which is what makes it read as the same material.
  canvas: "#ffffff",
  surface1: "#fcfcfc",

  // surface2/3/4 are deliberately NOT pinned: deriving them as ink mixes off
  // `surface1` keeps recessed fills (segmented wells, inputs, hover rows) in step
  // with --depth-step and the user's contrast setting.

  // Ink ramp — near-black anchor. Measured against Codex, body copy there is
  // essentially black and chrome labels sit around #282828; anchoring at
  // neutral-800 put every step here a visible notch lighter than both.
  inkBright: "#000000",
  ink: "#171717",
  inkSoft: "#4d4d4d",
  inkMuted: "#686868",
  // The quietest readable text rung still clears 4.5:1 on both canvas and
  // surface. Hierarchy comes from size/weight/placement, not illegible ink.
  inkFaint: "#747474",

  // Hairlines ARE the separation mechanism here, so they are tuned low: the seam
  // ring and the chrome divider both derive from `border`, and anything heavier
  // turns the UI into a wireframe.
  hairline: "rgb(0 0 0 / 0.05)",
  hairStrong: "rgb(0 0 0 / 0.14)",
  hairTertiary: "rgb(0 0 0 / 0.04)",
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
  // Primary CTA — inverting ink-on-white. Hover goes pure black.
  cta: {
    cta: "#171717",
    ctaHover: "#000000",
    ctaText: "#ffffff",
  },
});
