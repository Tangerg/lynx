// Catppuccin Mocha — canonical palette. Pins every surface step
// explicitly; the canonical feel relies on these exact tones.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  mauve: "#cba6f7",

  crust: "#11111b",
  mantle: "#181825",
  base: "#1e1e2e",
  surface0: "#313244",
  surface1: "#45475a",
  surface2: "#585b70",

  overlay0: "#6c7086",
  overlay1: "#7f849c",
  overlay2: "#9399b2",

  text: "#cdd6f4",
  subtext1: "#bac2de",
  subtext0: "#a6adc8",
};

export default defineThemePlugin({
  id: "catppuccin-mocha",
  label: "Catppuccin Mocha",
  scheme: "dark",
  order: 40,

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
    textFaint: "#8b91aa",
  },
  borders: {
    border: c.surface1,
    borderSoft: c.surface2,
    divider: c.overlay0,
    appDivider: c.surface1,
  },
  semantic: {
    negative: "#f38ba8",
    warning: "#fab387",
    info: "#89b4fa",
    success: "#a6e3a1",
  },
});
