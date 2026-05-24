// Solarized Dark — Ethan Schoonover's canonical palette. The base-*
// ladder is non-linear (perceptually equal steps on a yellow-blue
// axis, not a lightness ramp), so surface-2/3/4 are pinned explicitly.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  blue: "#268bd2",
  blueBorder: "#1e6fa6",
  bluePress: "#155383",

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
  id: "solarized-dark",
  label: "Solarized Dark",
  scheme: "dark",
  order: 30,

  brand: {
    accent: c.blue,
    accentBorder: c.blueBorder,
    accentPress: c.bluePress,
    textOnAccent: c.base3,
  },
  surfaces: {
    bg: c.base03,
    surface: c.base02,
    // Lifted surfaces extend the blue-grey axis (not a lightness step).
    surface2: "#0e4250",
    surface3: "#134e5e",
    surface4: "#185868",
  },
  ink: {
    text: c.base0,
    textBright: c.base3,
    textSoft: c.base1,
    // Bumped above base00 / base01 to clear WCAG AA on small body.
    textMuted: "#7a8e95",
    textFaint: "#687f86",
  },
  borders: {
    border: c.base01,
    borderSoft: c.base00,
    divider: c.base1,
    appDivider: c.base02,
  },
  semantic: {
    negative: "#dc322f",
    warning: "#cb4b16",
    info: "#2aa198",
    success: "#859900",
  },
});
