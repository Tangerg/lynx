// Solarized Light — same 8 accent hues as Dark, base-* ladder inverted.

import { defineThemePlugin } from "../themes/defineThemePlugin";

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

export default defineThemePlugin({
  id: "solarized-light",
  label: "Solarized Light",
  scheme: "light",
  order: 31,

  brand: {
    accent: c.blue,
    textOnAccent: c.base3,
  },
  surfaces: {
    bg: c.base2,
    surface: c.base3,
    // Solarized doesn't canonically define light lifted surfaces;
    // these step toward base2's hue.
    surface2: "#e7e0c8",
    surface3: "#d8d0b4",
    surface4: "#cac1a4",
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
    appDivider: c.base2,
  },
  semantic: {
    negative: "#dc322f",
    warning: "#cb4b16",
    info: "#2aa198",
    success: "#859900",
  },
});
