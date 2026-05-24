// Built-in plugin: Solarized Light theme.
//
// Mirror of Solarized Dark — same 8 accent hues, base-* ladder
// inverted. Same blue accent works on both schemes.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  blue:        "#268bd2",
  blueBorder:  "#1e6fa6",
  bluePress:   "#155383",

  base03: "#002b36",
  base02: "#073642",
  base01: "#586e75",
  base00: "#657b83",
  base0:  "#839496",
  base1:  "#93a1a1",
  base2:  "#eee8d5",  // light surface anchor
  base3:  "#fdf6e3",  // light canvas anchor
};

export default defineThemePlugin({
  id: "solarized-light",
  label: "Solarized Light",
  scheme: "light",
  order: 31,

  brand: {
    accent:       c.blue,
    accentBorder: c.blueBorder,
    accentPress:  c.bluePress,
    textOnAccent: c.base3,
  },
  surfaces: {
    bg:       c.base2,
    surface:  c.base3,
    // Derived "deeper" tones for hover / popover — Solarized doesn't
    // canonically define light lifted surfaces, so we step down toward
    // base2's hue.
    surface2: "#e7e0c8",
    surface3: "#d8d0b4",
    surface4: "#cac1a4",
  },
  ink: {
    text:       c.base00,
    textBright: c.base03,
    textSoft:   c.base01,
    // base1 #93a1a1 was ~3.2:1 on base3 (failing AA). Step toward base00.
    textMuted:  "#6f8388",
    // Derived "very faint" — base1 + base2 mix that reads ~4.6:1.
    textFaint:  "#7e8d8d",
  },
  borders: {
    border:     "#ddd6c1",
    borderSoft: "#c4bda4",
    divider:    c.base1,
    appDivider: c.base2,
  },
  semantic: {
    // Solarized canonical hues — work on both schemes per the spec
    negative: "#dc322f",
    warning:  "#cb4b16",
    info:     "#2aa198",
    success:  "#859900",
  },
});
