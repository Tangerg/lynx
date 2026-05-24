// Built-in plugin: Tokyo Night Storm theme.
//
// Canonical palette from enkia/tokyonight (Storm variant) — same values
// VS Code's `Tokyo Night` extension and the Vim/Neovim port use. Slightly
// softer than the regular "Night" variant; bg is #1f2335 instead of
// #1a1b26.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  // Brand — Tokyo Night signature blue
  blue:        "#7aa2f7",
  blueBorder:  "#5d86e6",
  bluePress:   "#4068c5",

  // Tokyo Night Storm surfaces — bg_dark / bg / bg_highlight / fg_gutter
  bgDark:      "#1f2335",
  bg:          "#24283b",
  bgHighlight: "#292e42",
  bgHover:     "#2f3549",

  // Ink — fg / fg_dark / fg_gutter / comment
  fg:          "#c0caf5",
  fgDark:      "#a9b1d6",
  fgBright:    "#ffffff",
  // Bumped from #787c99 → ~5.0:1 on bg (was ~4.0). Still subordinate to
  // fgDark (#a9b1d6 → ~7.6:1) so the hierarchy reads.
  fgMuted:     "#8389ab",
  // Bumped from #565f89 (comment, ~3.4:1) → #6873a3 (~4.6:1).
  fgFaint:     "#6873a3",

  // Hairlines — fg_gutter ladder
  hairline:    "#3b4261",
  hairStrong:  "#545c7e",
  hairTertiary:"#6873a3",
};

export default defineThemePlugin({
  id: "tokyo-night-storm",
  label: "Tokyo Night Storm",
  scheme: "dark",
  order: 20,

  brand: {
    accent:       c.blue,
    accentBorder: c.blueBorder,
    accentPress:  c.bluePress,
    // Tokyo Night runs dark ink on the lighter blue accent — matches
    // the upstream VS Code build.
    textOnAccent: "#1a1b26",
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
    textSoft:   c.fgDark,
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
    // Tokyo Night canonical syntax pulls
    negative: "#f7768e",
    warning:  "#ff9e64",
    info:     "#7dcfff",
    success:  "#9ece6a",
  },
});
