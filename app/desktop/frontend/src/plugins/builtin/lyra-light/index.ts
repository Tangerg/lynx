// Built-in plugin: Lyra Light — Vercel dashboard inspired.
//
// Bright canvas + white surface. CTAs go pure black-on-white (Vercel
// signature), decoupled from the accent — so accent can stay reserved
// for "live state" indicators without forcing primary buttons green.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  // Brand — Lyra green, dimmed for white background
  green:       "#15883e",
  greenBorder: "#117134",
  greenPress:  "#0c5d2a",

  // Surface anchors — Vercel-style #fafafa canvas / #ffffff surface
  canvas:      "#fafafa",
  surface1:    "#ffffff",

  // Ink — Vercel #171717 / #4d4d4d / #5e5e5e ladder.
  // text-muted + text-faint calibrated for WCAG AA on small body sizes
  // (~6.8:1 / ~4.9:1 on the #fafafa canvas).
  inkBright:   "#000000",
  ink:         "#171717",
  inkSoft:     "#4d4d4d",
  inkMuted:    "#5e5e5e",
  inkFaint:    "#707070",

  // Hairlines — Vercel #ebebeb / #d4d4d6 / #a1a1a1 ladder
  hairline:    "#ebebeb",
  hairStrong:  "#d4d4d6",
  hairTertiary:"#a1a1a1",
};

export default defineThemePlugin({
  id: "light",
  label: "Light",
  scheme: "light",
  order: 1,

  brand: {
    accent:       c.green,
    accentBorder: c.greenBorder,
    accentPress:  c.greenPress,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    bg:      c.canvas,
    surface: c.surface1,
  },
  ink: {
    text:       c.ink,
    textBright: c.inkBright,
    textSoft:   c.inkSoft,
    textMuted:  c.inkMuted,
    textFaint:  c.inkFaint,
  },
  borders: {
    border:     c.hairline,
    borderSoft: c.hairStrong,
    divider:    c.hairTertiary,
    appDivider: c.hairline,
  },
  semantic: {
    negative: "#ee0000",
    warning:  "#f5a623",
    info:     "#0070f3",
    success:  "#15883e",
  },
  // Vercel signature CTA — pure black on white. Decoupled from accent
  // so accent stays reserved for live indicators (running pill, focus
  // ring, active tab line).
  cta: {
    cta:      "#000000",
    ctaHover: "#222222",
    ctaText:  "#ffffff",
  },
});
