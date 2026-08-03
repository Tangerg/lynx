// Light — the JetBrains tool-window palette, light scheme.
//
// Same region algorithm as the dark scheme, mirrored: the reading plane is the
// brightest surface and the chrome around it steps DOWN. Depth is the value delta
// plus a directional cast at each seam, never a border.
//
// The card anchor is where the two schemes stop being a single mix. On dark a card
// lifts toward the ink; here it lifts away from it, to pure white over an off-white
// plane. That is why `elevated` is an anchor and not a ladder rung.
//
// Semantic hues follow the reference language but are one step deeper than it: the
// reference's greens and ambers land at 3.4–3.9:1 as text on these surfaces, and a
// status word nobody can read is not a status. Hue family preserved, luminance
// pulled until each clears 4.5:1 on the darkest surface it can sit on.

import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  // Deeper than the dark scheme's #3574f0 for the same reason as the semantics —
  // the reference blue reads at 3.8:1 on this chrome. Same hue, AA-clean.
  accent: "#2b5fd0",

  canvas: "#fbfbfc",
  surface1: "#f0f1f3",
  sunken: "#eef0f2",

  inkBright: "#000000",
  ink: "#1e1f22",
  inkSoft: "#3d4147",
  inkMuted: "#5a5d63",
  inkFaint: "#63666d",

  hairline: "#dfe1e5",
  hairStrong: "#c7cad0",
  hairTertiary: "rgb(0 0 0 / 0.07)",
};

export default defineColorThemePlugin({
  id: "light",
  label: "Light",
  scheme: "light",
  order: 0,

  brand: {
    accent: c.accent,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    bg: c.canvas,
    surface: c.surface1,
    elevated: "#ffffff",
    sunken: c.sunken,
  },
  ink: {
    text: c.ink,
    textBright: c.inkBright,
    textSoft: c.inkSoft,
    textMuted: c.inkMuted,
    textFaint: c.inkFaint,
  },
  borders: {
    border: c.hairline,
    borderSoft: c.hairStrong,
    divider: c.hairTertiary,
  },
  semantic: {
    negative: "#b0342b",
    warning: "#84610e",
    info: c.accent,
    success: "#2a713e",
  },
  // The accent itself. Every light-scheme accent is already tuned for a white
  // ground and carries white label text at 5.6:1 or better, so the extra shade
  // the dark scheme needs would only push the primary button toward navy.
  cta: {
    cta: "var(--color-accent)",
    ctaHover: "var(--color-accent-border)",
    ctaText: "var(--color-text-on-accent)",
  },
});
