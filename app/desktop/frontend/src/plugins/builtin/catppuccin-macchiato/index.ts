// Catppuccin Macchiato — canonical palette. Sits between Mocha (darkest)
// and Frappé; warmer, slightly bluer base than Mocha. Pins every surface
// step explicitly; the canonical feel relies on these exact tones.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  mauve: "#c6a0f6",

  crust: "#181926",
  mantle: "#1e2030",
  base: "#24273a",
  surface0: "#363a4f",
  surface1: "#494d64",
  surface2: "#5b6078",

  overlay0: "#6e738d",
  overlay1: "#8087a2",
  overlay2: "#939ab7",

  text: "#cad3f5",
  subtext1: "#b8c0e0",
  subtext0: "#a5adcb",
};

export default defineThemePlugin({
  id: "catppuccin-macchiato",
  label: "Catppuccin Macchiato",
  scheme: "dark",
  order: 41,

  brand: {
    accent: c.mauve,
    textOnAccent: c.base,
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
    textBright: "#ffffff",
    textSoft: c.subtext1,
    textMuted: c.subtext0,
    // Bumped above overlay1 to clear WCAG AA on small body.
    textFaint: "#8d94af",
  },
  borders: {
    border: c.surface1,
    borderSoft: c.surface2,
    divider: c.overlay0,
    appDivider: c.surface1,
  },
  semantic: {
    negative: "#ed8796",
    warning: "#f5a97f",
    info: "#8aadf4",
    success: "#a6da95",
  },
});
