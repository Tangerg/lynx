// Built-in plugin: Lyra Dark — the system default theme.
//
// Synthesis of Linear (canvas / surface / hairline) + Vercel (Geist
// typography / stacked elevation). The values here are the source of
// truth — tokens.css's `:root` block only carries first-paint
// fallbacks for the brief window before this plugin's setup runs.

import { defineThemePlugin } from "../themes/defineThemePlugin";

// Named palette — local consts let us write the value once and reuse it
// across sections. (Pi-mono uses a `vars` JSON indirection for this;
// TypeScript const does the same thing with zero runtime cost.)
const c = {
  // Brand — Lyra signature green
  green:       "#1ed760",
  greenBorder: "#1db954",
  greenPress:  "#169c46",

  // Surface anchors
  canvas:      "#010102",  // page background — faint blue tint, not pure black
  surface1:    "#181a1d",  // panel / sidebar / message bubble

  // Ink
  inkBright:   "#ffffff",
  ink:         "#f7f8f8",
  inkSoft:     "#d0d6e0",
  // Calibrated for WCAG AA at 11-12px sizes on canvas (~5.6:1 / ~4.7:1).
  inkMuted:    "#9ea3ac",
  inkFaint:    "#76787e",

  // Hairlines — literal hex (DESIGN.md §2: precise > alpha-blended)
  hairline:    "#23252a",
  hairStrong:  "#34343a",
  hairTertiary:"#3e3e44",
};

export default defineThemePlugin({
  id: "dark",
  label: "Dark",
  scheme: "dark",
  order: 0,

  brand: {
    accent:       c.green,
    accentBorder: c.greenBorder,
    accentPress:  c.greenPress,
    textOnAccent: "#000000", // black ink reads on bright green
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
    success:  "#27a644",
  },
});
