// Tokyo Night Storm — canonical enkia/tokyonight Storm variant.

import { defineThemePlugin } from "../kit/defineThemePlugin";

const c = {
  blue: "#7aa2f7",

  bgDark: "#1f2335",
  bg: "#24283b",
  bgHighlight: "#292e42",
  bgHover: "#2f3549",

  fg: "#c0caf5",
  fgDark: "#a9b1d6",
  fgBright: "#ffffff",
  // Bumped above the original #787c99 (~4.0:1) / #565f89 (~3.4:1) to
  // clear WCAG AA on 11-12px text against the Storm bg.
  fgMuted: "#8389ab",
  fgFaint: "#6873a3",

  hairline: "#3b4261",
  hairStrong: "#545c7e",
  hairTertiary: "#6873a3",
};

export default defineThemePlugin({
  id: "tokyo-night-storm",
  label: "Tokyo Night Storm",
  scheme: "dark",
  order: 20,

  brand: {
    accent: c.blue,
    // Dark ink on the lighter blue accent — matches the upstream build.
    textOnAccent: "#1a1b26",
  },
  surfaces: {
    bg: c.bgDark,
    surface: c.bg,
    surface2: c.bgHighlight,
    surface3: c.bgHover,
  },
  ink: {
    text: c.fg,
    textBright: c.fgBright,
    textSoft: c.fgDark,
    textMuted: c.fgMuted,
    textFaint: c.fgFaint,
  },
  borders: {
    border: c.hairline,
    borderSoft: c.hairStrong,
    divider: c.hairTertiary,
  },
  semantic: {
    negative: "#f7768e",
    warning: "#ff9e64",
    info: "#7dcfff",
    success: "#9ece6a",
  },
});
