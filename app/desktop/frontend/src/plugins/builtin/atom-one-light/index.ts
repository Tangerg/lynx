// Built-in plugin: Atom One Light theme.
//
// Canonical palette from the `one-light-syntax` Atom package + VS Code's
// `Atom One Light` theme.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  // Brand — One Light cursor blue (slightly cooler than One Dark)
  blue: "#526fff",
  blueBorder: "#4060e8",
  bluePress: "#2d4ac6",

  // Atom One Light canonical surfaces
  panel: "#f0f0f1", // sidebar / panel
  editor: "#fafafa", // editor
  sel1: "#e5e5e6", // selection bg
  sel2: "#d4d4d6",
  sel3: "#c5c5c6",

  // Ink — `mono-1` body, `mono-2/3` muted/faint
  fg: "#383a42",
  fgBright: "#000000",
  // Bumped from #696c77 → reads ~4.6:1 on #fafafa surface (was failing AA).
  fgMuted: "#5b5e66",
  // Bumped from #a0a1a7 (comment) which was ~2.7:1. New ~4.6:1.
  fgFaint: "#74757c",

  // Hairlines
  hairline: "#e5e5e6",
  hairStrong: "#c5c5c6",
  hairTertiary: "#a0a1a7",
};

export default defineThemePlugin({
  id: "atom-one-light",
  label: "Atom One Light",
  scheme: "light",
  order: 11,

  brand: {
    accent: c.blue,
    accentBorder: c.blueBorder,
    accentPress: c.bluePress,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    bg: c.panel,
    surface: c.editor,
    surface2: c.sel1,
    surface3: c.sel2,
    surface4: c.sel3,
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
    negative: "#e45649",
    warning: "#986801",
    info: "#4078f2",
    success: "#50a14f",
  },
});
