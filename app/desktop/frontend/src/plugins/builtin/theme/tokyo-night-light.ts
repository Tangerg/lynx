// Tokyo Night Light — canonical Day variant. Cool-grey bg ladder
// instead of pure white so the saturated blue accent doesn't scream.

import { defineThemePlugin } from "./kit/defineThemePlugin";

const c = {
  blue: "#34548a",

  bgDark: "#cbccd1",
  bg: "#d5d6db",
  bgHighlight: "#c4c8d8",
  bgHover: "#b8bdd0",

  fg: "#343b58",
  fgBright: "#1a1b26",
  // Bumped above the original #4c505e / #848cb5 to clear WCAG AA.
  fgMuted: "#454960",
  fgFaint: "#5e6585",

  hairline: "#a8aecb",
  hairStrong: "#909bbe",
  hairTertiary: "#7882a8",
};

export default defineThemePlugin({
  id: "tokyo-night-light",
  label: "Tokyo Night Light",
  scheme: "light",
  order: 21,

  brand: {
    accent: c.blue,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    bg: c.bgDark,
    surface: c.bg,
    surface2: c.bgHighlight,
    surface3: c.bgHover,
  },
  ink: {
    text: c.fg,
    textBright: c.fgBright,
    textSoft: c.fg,
    textMuted: c.fgMuted,
    textFaint: c.fgFaint,
  },
  borders: {
    border: c.hairline,
    borderSoft: c.hairStrong,
    divider: c.hairTertiary,
    appDivider: c.hairline,
  },
  semantic: {
    // Tokyo Night Day syntax pulls — desaturated for the light surface
    negative: "#8c4351",
    warning: "#965027",
    info: "#0f4b6e",
    success: "#485e30",
  },
});
