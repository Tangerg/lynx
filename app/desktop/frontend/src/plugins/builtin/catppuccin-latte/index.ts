// Built-in plugin: Catppuccin Latte theme.
//
// The light counterpart to Mocha. Latte's mauve is far more saturated
// than Mocha's — it has to bite against the bright surface.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  // Brand — Latte's saturated mauve
  mauve:       "#8839ef",
  mauveBorder: "#6f25d4",
  mauvePress:  "#5817b3",

  // Latte base ladder
  base:        "#eff1f5",
  mantle:      "#e6e9ef",
  surface0:    "#ccd0da",
  surface1:    "#bcc0cc",
  surface2:    "#acb0be",

  // Overlay / muted
  overlay0:    "#9ca0b0",
  overlay1:    "#8c8fa1",

  // Text
  text:        "#4c4f69",
  subtext1:    "#5c5f77",
  subtext0:    "#6c6f85",
};

export default defineThemePlugin({
  id: "catppuccin-latte",
  label: "Catppuccin Latte",
  scheme: "light",
  order: 41,

  brand: {
    accent:       c.mauve,
    accentBorder: c.mauveBorder,
    accentPress:  c.mauvePress,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    bg:       c.mantle,
    surface:  c.base,
    surface2: c.surface0,
    surface3: c.surface1,
    surface4: c.surface2,
  },
  ink: {
    text:       c.text,
    textBright: "#000000",
    textSoft:   c.subtext1,
    textMuted:  c.subtext0,
    // Bumped from overlay1 #8c8fa1 (~3.4:1 on base) → #75788a (~4.7:1).
    textFaint:  "#75788a",
  },
  borders: {
    border:     c.surface0,
    borderSoft: c.surface1,
    divider:    c.surface2,
    appDivider: c.surface0,
  },
  semantic: {
    // Canonical Catppuccin Latte
    negative: "#d20f39",
    warning:  "#fe640b",
    info:     "#1e66f5",
    success:  "#40a02b",
  },
});
