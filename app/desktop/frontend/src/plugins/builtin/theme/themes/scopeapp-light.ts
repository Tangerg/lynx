// Light — the JetBrains tool-window palette, light scheme.
//
// The reading plane is the brightest surface and everything else steps DOWN from
// it. Depth is the value delta plus a directional cast at each seam, never a
// border.
//
// The plane is pure white, so an object ON it is a step IN rather than a step up:
// a card at #ffffff over an off-white plane was 1.2 L of delta with no hue at all,
// and a card that faint is held up entirely by its shadow. Objects step toward the
// chrome, which is also why a card inside the drawer now reads as lifted instead of
// as a hole punched to white.
//
// ONE HUE, CHROMA BY AREA. Every neutral sits on the accent's own hue (263°) and
// carries chroma in inverse proportion to how much of the screen it covers: the
// plane none, the chrome 0.006, a card 0.008, a well 0.016. Below roughly C 0.005 a
// grey's hue is not addressable in 8-bit sRGB — one byte swings it 20–40° — so the
// old ramp drifted across 248…286° purely as quantisation noise, and a hue nobody
// chose is what "dirty grey" means. Raising chroma is what makes the family read as
// material instead of as grime; keeping it low on the large areas is what keeps the
// same decision from reading as a blue tint.
//
// Semantic hues follow the reference language but are one step deeper than it: the
// reference's greens and ambers land at 3.4–3.9:1 as text on these surfaces, and a
// status word nobody can read is not a status. Hue family preserved, luminance
// pulled until each clears 4.5:1 on the darkest surface it can sit on.

import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  // Deeper than the dark scheme's #3574f0 for the same reason as the semantics —
  // the reference blue reads at 3.8:1 on this chrome. Same hue, AA-clean. Its hue
  // is the one every neutral above is tuned to.
  accent: "#2b5fd0",

  canvas: "#ffffff",
  card: "#f7faff",
  // The chrome sits 2.7 L under the plane, not 4.2. At the deeper step the column read
  // as grey rather than as paper of a different weight — the reference keeps its sidebar
  // at 248 against a 255 plane and lets the seam carry the boundary, which is the trade
  // this makes: a lighter panel and a hairline that means it.
  surface1: "#f4f6fa",
  sunken: "#eaf0fb",

  inkBright: "#000000",
  ink: "#1e1f22",
  inkSoft: "#3d4147",
  inkMuted: "#5a5d63",
  inkFaint: "#63666d",

  // Three weights, and all three are the reference's ink percentages rather than
  // picked greys: 5 / 8 / 12 percent of the ink over the plane, which lands at
  // 244 / 237 / 228 against 255. They had been 226 / 204 — a seam 24 levels
  // heavier than the deepest line the reference draws anywhere, which is what
  // made a window of tool panels read as a wireframe of boxes rather than as
  // paper of different weights. The value delta between the panels was never the
  // problem: ours is 255/247 against the reference's 255/249.
  hairline: "#e9ecf2",
  hairStrong: "#dee2eb",
  hairTertiary: "rgb(0 0 0 / 0.05)",
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
    elevated: c.card,
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
  // Where each neutral sits, so the shell can rewrite them onto the live accent. The
  // hexes above are the same family at the default accent — they are what a cold boot
  // paints before this runs. See kit/accentTint for the rule and why it is Material's.
  neutralSteps: {
    surface: { l: 97.3, c: 0.006 },
    elevated: { l: 98.4, c: 0.008 },
    sunken: { l: 95.4, c: 0.016 },
    border: { l: 94.3, c: 0.009 },
    borderSoft: { l: 91.2, c: 0.013 },
  },
  cta: {
    cta: "var(--color-accent)",
    ctaHover: "var(--color-accent-border)",
    ctaText: "var(--color-text-on-accent)",
  },
});
