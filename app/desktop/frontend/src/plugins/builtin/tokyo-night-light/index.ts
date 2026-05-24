// Built-in plugin: Tokyo Night Light theme.
//
// Canonical palette from enkia/tokyonight (Day/Light variant) —
// intentionally muted compared to other light themes. Bg ladder uses
// soft cool greys rather than pure white so the accent blues don't
// scream against the surface.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  // Brand — Tokyo Night Day's darker, more saturated blue
  blue:        "#34548a",
  blueBorder:  "#2a4373",
  bluePress:   "#1f335a",

  // Tokyo Night Day surfaces — bg_dark / bg / bg_highlight
  bgDark:      "#cbccd1",
  bg:          "#d5d6db",
  bgHighlight: "#c4c8d8",
  bgHover:     "#b8bdd0",

  // Ink — fg / fg_dark / comment
  fg:          "#343b58",
  fgBright:    "#1a1b26",
  // Bumped from #4c505e → reads ~5.4:1 on #d5d6db (was just above AA).
  fgMuted:     "#454960",
  // Bumped from #848cb5 (~2.7:1 on #d5d6db) → #5e6585 (~4.6:1).
  fgFaint:     "#5e6585",

  // Hairlines — fg_gutter ladder
  hairline:    "#a8aecb",
  hairStrong:  "#909bbe",
  hairTertiary:"#7882a8",
};

export default defineThemePlugin({
  id: "tokyo-night-light",
  label: "Tokyo Night Light",
  scheme: "light",
  order: 21,

  brand: {
    accent:       c.blue,
    accentBorder: c.blueBorder,
    accentPress:  c.bluePress,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    bg:       c.bgDark,
    surface:  c.bg,
    surface2: c.bgHighlight,
    surface3: c.bgHover,
  },
  ink: {
    text:       c.fg,
    textBright: c.fgBright,
    textSoft:   c.fg,
    textMuted:  c.fgMuted,
    textFaint:  c.fgFaint,
  },
  borders: {
    border:     c.hairline,
    borderSoft: c.hairStrong,
    divider:    c.hairTertiary,
    appDivider: c.hairline,
  },
  semantic: {
    // Tokyo Night Day syntax pulls — desaturated for the light surface
    negative: "#8c4351",
    warning:  "#965027",
    info:     "#0f4b6e",
    success:  "#485e30",
  },
});
