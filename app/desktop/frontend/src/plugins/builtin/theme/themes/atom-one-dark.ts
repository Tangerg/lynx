// Atom One Dark — canonical palette from one-dark-syntax / One Dark Pro.

import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  blue: "#528bff",

  well: "#16191e",
  panel: "#1c2026",
  editor: "#282c34",
  selection1: "#2c313a",
  selection2: "#2f343d",
  selection3: "#323843",

  fg: "#abb2bf",
  fgBright: "#ffffff",
  fgMuted: "#828997",
  // Original #5c6370 (comment) was 3.8:1 on the editor bg — failed AA.
  fgFaint: "#6e7480",

  hairline: "#3a3f4b",
  hairStrong: "#4b5263",
  hairTertiary: "#5c6370",
};

export default defineColorThemePlugin({
  id: "atom-one-dark",
  label: "Atom One Dark",
  scheme: "dark",
  order: 10,

  brand: {
    accent: c.blue,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    // Atom's selection ladder is non-linear; pin the canonical tones.
    bg: c.panel,
    surface: c.editor,
    elevated: c.editor,
    sunken: c.well,
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
  },
  semantic: {
    negative: "#e06c75",
    warning: "#d19a66",
    info: "#61afef",
    success: "#98c379",
  },
});
