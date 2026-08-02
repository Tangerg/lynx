// Catppuccin Latte — light counterpart to Mocha. Saturated mauve to
// bite against the bright surface.

import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  mauve: "#8839ef",

  base: "#eff1f5",
  mantle: "#e6e9ef",
  crust: "#dce0e8",
  surface0: "#ccd0da",
  surface1: "#bcc0cc",
  surface2: "#acb0be",

  overlay0: "#9ca0b0",
  overlay1: "#8c8fa1",

  text: "#4c4f69",
  subtext1: "#5c5f77",
  subtext0: "#6c6f85",
};

export default defineColorThemePlugin({
  id: "catppuccin-latte",
  label: "Catppuccin Latte",
  scheme: "light",
  order: 42,

  brand: {
    accent: c.mauve,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    // Base is the reading plane, mantle the chrome. Inverted from the dark
    // variant on purpose: in a light scheme the surface you read on has to be the
    // brightest one, or the chrome shouts over the transcript.
    bg: c.base,
    surface: c.mantle,
    elevated: "#ffffff",
    sunken: c.crust,
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
  },
  semantic: {
    negative: "#d20f39",
    warning: "#fe640b",
    info: "#1e66f5",
    success: "#40a02b",
  },
});
