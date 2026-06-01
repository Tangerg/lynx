// Atom One Light — canonical one-light-syntax palette.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  blue: "#526fff",

  panel: "#f0f0f1",
  editor: "#fafafa",
  sel1: "#e5e5e6",
  sel2: "#d4d4d6",
  sel3: "#c5c5c6",

  fg: "#383a42",
  fgBright: "#000000",
  // Bumped above the original #696c77 / #a0a1a7 to clear WCAG AA.
  fgMuted: "#5b5e66",
  fgFaint: "#74757c",

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
