// Solarized Light — same 8 accent hues as Dark, base-* ladder inverted.

import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  blue: "#268bd2",

  base03: "#002b36",
  base02: "#073642",
  base01: "#586e75",
  base00: "#657b83",
  base0: "#839496",
  base1: "#93a1a1",
  base2: "#eee8d5",
  base3: "#fdf6e3",
};

export default defineColorThemePlugin({
  id: "solarized-light",
  label: "Solarized Light",
  scheme: "light",
  order: 31,

  brand: {
    accent: c.blue,
    textOnAccent: c.base3,
  },
  surfaces: {
    // base3 is Solarized's editor tone, so it is the plane; base2 is the
    // highlight tone and frames it. Solarized names nothing above base3, so the
    // card lifts a hair toward warm white rather than to a neutral #fff, which
    // would break the palette's yellow cast.
    bg: c.base3,
    surface: c.base2,
    elevated: "#fffdf4",
    sunken: c.base2,
  },
  ink: {
    text: c.base00,
    textBright: c.base03,
    textSoft: c.base01,
    // Bumped above base1 to clear WCAG AA on small body.
    textMuted: "#6f8388",
    textFaint: "#7e8d8d",
  },
  borders: {
    border: "#ddd6c1",
    borderSoft: "#c4bda4",
    divider: c.base1,
  },
  semantic: {
    negative: "#dc322f",
    warning: "#cb4b16",
    info: "#2aa198",
    success: "#859900",
  },
});
