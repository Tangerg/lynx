// Built-in plugin: Solarized Dark theme.
//
// Ethan Schoonover's Solarized. Dark and Light share the same 8 accent
// hues — only the base-* ladder inverts.
//
// Solarized's base ladder is non-linear (base03 → base02 → base01 →
// base00 are perceptually equal-distance steps on a yellow-blue axis,
// not a simple lightness ramp), so we pin surface-2/3/4 explicitly
// rather than let color-mix derive them.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  // Brand — Solarized blue
  blue:        "#268bd2",
  blueBorder:  "#1e6fa6",
  bluePress:   "#155383",

  // Solarized base ladder — canonical hexes
  base03: "#002b36",  // canvas
  base02: "#073642",  // surface
  base01: "#586e75",  // comments / muted
  base00: "#657b83",  // body / soft
  base0:  "#839496",  // default text
  base1:  "#93a1a1",  // emphasis
  base2:  "#eee8d5",  // light surface
  base3:  "#fdf6e3",  // light canvas
};

export default defineThemePlugin({
  id: "solarized-dark",
  label: "Solarized Dark",
  scheme: "dark",
  order: 30,

  brand: {
    accent:       c.blue,
    accentBorder: c.blueBorder,
    accentPress:  c.bluePress,
    textOnAccent: c.base3,
  },
  surfaces: {
    bg:       c.base03,
    surface:  c.base02,
    // Solarized's "lifted" surfaces extend the same blue-grey axis,
    // not a lightness step. Manual ladder keeps the canonical feel.
    surface2: "#0e4250",
    surface3: "#134e5e",
    surface4: "#185868",
  },
  ink: {
    text:       c.base0,
    textBright: c.base3,
    textSoft:   c.base1,
    // Bumped from base00 (#657b83 → ~4.3:1 on base03) → #7a8e95 (~5.1:1).
    textMuted:  "#7a8e95",
    // Bumped from base01 (#586e75 → ~3.1:1) → #687f86 (~4.6:1).
    textFaint:  "#687f86",
  },
  borders: {
    border:     c.base01,
    borderSoft: c.base00,
    divider:    c.base1,
    appDivider: c.base02,
  },
  semantic: {
    // Solarized canonical accent hues — already plenty of contrast
    negative: "#dc322f",
    warning:  "#cb4b16",
    info:     "#2aa198",
    success:  "#859900",
  },
});
