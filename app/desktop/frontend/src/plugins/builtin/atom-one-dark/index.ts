// Built-in plugin: Atom One Dark theme.
//
// Canonical palette from the `one-dark-syntax` Atom package + VS Code's
// `One Dark Pro` extension (identical values across both). Accent =
// Atom's cursor blue, which doubles as the activity-bar marker.

import { defineThemePlugin } from "../themes/defineThemePlugin";

const c = {
  // Brand — Atom cursor blue
  blue:        "#528bff",
  blueBorder:  "#4078e6",
  bluePress:   "#2f63cc",

  // Atom One Dark canonical surfaces
  panel:       "#1c2026", // panel background — sidebar / inactive editor
  editor:      "#282c34", // editor background — primary "page" surface

  // Selection / lifted surface ladder — Atom uses #2c313a (sel-bg) and
  // #2f343d (sel-bg+) for hover / active rows. Explicit ladder keeps the
  // depth feel matching the original IDE.
  selection1:  "#2c313a",
  selection2:  "#2f343d",
  selection3:  "#323843",

  // Ink — One Dark `fg`, `text-light`, `mono-1`, `mono-2`, `mono-3`
  fg:          "#abb2bf",
  fgBright:    "#ffffff",
  fgMuted:     "#828997",
  // Bumped from the original #5c6370 — that's the comment color which
  // failed WCAG AA at 11-12px (~3.8:1 on #282c34). 6e7480 reads ~4.6:1.
  fgFaint:     "#6e7480",

  // Hairlines — derived from selection ladder so borders sit cleanly on
  // either panel or editor surface.
  hairline:    "#3a3f4b",
  hairStrong:  "#4b5263",
  hairTertiary:"#5c6370",
};

export default defineThemePlugin({
  id: "atom-one-dark",
  label: "Atom One Dark",
  scheme: "dark",
  order: 10,

  brand: {
    accent:       c.blue,
    accentBorder: c.blueBorder,
    accentPress:  c.bluePress,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    bg:       c.panel,
    surface:  c.editor,
    // Atom's selection ladder is non-linear; surface-2/3/4 are pinned
    // to the canonical sel-bg / sel-bg+ / sel-bg++ tones rather than
    // color-mix derivations.
    surface2: c.selection1,
    surface3: c.selection2,
    surface4: c.selection3,
  },
  ink: {
    text:       c.fg,
    textBright: c.fgBright,
    textSoft:   c.fg,
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
    // One Dark syntax pulls — Atom's canonical semantic colors
    negative: "#e06c75",
    warning:  "#d19a66",
    info:     "#61afef",
    success:  "#98c379",
  },
});
