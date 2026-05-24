// Built-in plugin: Catppuccin Mocha theme.
//
// Canonical palette from catppuccin/catppuccin (Mocha — popular dark
// variant). Default accent = mauve, matching catppuccin/vscode.
//
// Catppuccin has its own well-defined surface ladder (mantle / base /
// surface0/1/2 / overlay0/1) so we pin every step explicitly instead
// of letting color-mix derive them — the canonical feel relies on
// these exact tones.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  // Brand — Catppuccin mauve
  mauve:       "#cba6f7",
  mauveBorder: "#b18cf3",
  mauvePress:  "#9572ed",

  // Mocha base ladder
  crust:       "#11111b",
  mantle:      "#181825",
  base:        "#1e1e2e",
  surface0:    "#313244",
  surface1:    "#45475a",
  surface2:    "#585b70",

  // Overlay ladder (for dividers / muted text)
  overlay0:    "#6c7086",
  overlay1:    "#7f849c",
  overlay2:    "#9399b2",

  // Text ladder — text / subtext1 / subtext0
  text:        "#cdd6f4",
  subtext1:    "#bac2de",
  subtext0:    "#a6adc8",
};

export default defineThemePlugin({
  id: "catppuccin-mocha",
  label: "Catppuccin Mocha",
  scheme: "dark",
  order: 40,

  brand: {
    accent:       c.mauve,
    accentBorder: c.mauveBorder,
    accentPress:  c.mauvePress,
    textOnAccent: c.base,
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
    textBright: "#ffffff",
    textSoft:   c.subtext1,
    textMuted:  c.subtext0,
    // Bumped from overlay1 #7f849c (~4.0:1 on base) → #8b91aa (~4.7:1).
    textFaint:  "#8b91aa",
  },
  borders: {
    border:     c.surface1,
    borderSoft: c.surface2,
    divider:    c.overlay0,
    appDivider: c.surface1,
  },
  semantic: {
    // Canonical Catppuccin Mocha — flamingo / peach / sapphire / green
    negative: "#f38ba8",
    warning:  "#fab387",
    info:     "#89b4fa",
    success:  "#a6e3a1",
  },
});
