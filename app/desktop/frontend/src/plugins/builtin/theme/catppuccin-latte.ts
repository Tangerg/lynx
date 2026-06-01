// Catppuccin Latte — light counterpart to Mocha. Saturated mauve to
// bite against the bright surface.

import { defineThemePlugin } from "./kit/defineThemePlugin";

const c = {
  mauve: "#8839ef",

  base: "#eff1f5",
  mantle: "#e6e9ef",
  surface0: "#ccd0da",
  surface1: "#bcc0cc",
  surface2: "#acb0be",

  overlay0: "#9ca0b0",
  overlay1: "#8c8fa1",

  text: "#4c4f69",
  subtext1: "#5c5f77",
  subtext0: "#6c6f85",
};

export default defineThemePlugin({
  id: "catppuccin-latte",
  label: "Catppuccin Latte",
  scheme: "light",
  order: 42,

  brand: {
    accent: c.mauve,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    bg: c.mantle,
    surface: c.base,
    surface2: c.surface0,
    surface3: c.surface1,
    surface4: c.surface2,
  },
  ink: {
    text: c.text,
    textBright: "#000000",
    textSoft: c.subtext1,
    textMuted: c.subtext0,
    // Bumped above overlay1 to clear WCAG AA on small body.
    textFaint: "#75788a",
  },
  borders: {
    border: c.surface0,
    borderSoft: c.surface1,
    divider: c.surface2,
    appDivider: c.surface0,
  },
  semantic: {
    negative: "#d20f39",
    warning: "#fe640b",
    info: "#1e66f5",
    success: "#40a02b",
  },
});
