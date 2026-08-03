// Dark — the JetBrains tool-window palette.
//
// Three region materials, no lines between them. The reading plane is the DARKEST
// surface and the chrome around it steps up: that inversion is the whole idiom —
// an IDE puts you inside the editor and frames it with tool windows, where a web
// app would float a white card on grey. Depth comes from the value delta plus a
// directional cast at each seam (the visual style owns both), never from a border.
//
// Cards sit at the same value as the chrome, so an object on the plane reads as
// lifted while the same object inside a tool window reads as flush. `sunken` is
// well below the plane and carries every recessed well: code, terminal, diff.
//
// ONE HUE, CHROMA BY AREA — the same policy as the light scheme, and the same
// reason: under roughly C 0.005 a grey's hue is quantisation noise rather than a
// decision, which is how a neutral ramp comes out reading as grime. Every neutral
// here sits on 263° with chroma in inverse proportion to area (plane 0.008, chrome
// 0.010, well 0.015). The moves are single bytes — near-black has little room —
// but they put both schemes on one family instead of two.

import { defineColorThemePlugin } from "../kit/defineColorThemePlugin";

const c = {
  // The one accent. Live indicators, progress, primary fills, focus, links. Its
  // hue is what every neutral below is tuned to.
  accent: "#3574f0",

  // Reading plane, region chrome, and the well beneath both.
  canvas: "#1d1f23",
  surface1: "#2a2d32",
  sunken: "#14181f",

  // surface2/3/4 derive as ink mixes off `surface1` — those are the chip rungs
  // (badge, inline code, kbd, selected row), not region materials.

  inkBright: "#ffffff",
  ink: "#e3e5e9",
  inkSoft: "#c6c9cf",
  inkMuted: "#aaaeb5",
  inkFaint: "#95999f",

  // A control's edge is a real line here; a REGION's edge is not. Region
  // separation is the visual style's cast, so nothing in this ramp is ever
  // stretched into a pane divider.
  hairline: "#3a3d42",
  hairStrong: "#4b4e54",
  hairTertiary: "rgb(255 255 255 / 0.07)",
};

export default defineColorThemePlugin({
  id: "dark",
  label: "Dark",
  scheme: "dark",
  order: 0,

  brand: {
    accent: c.accent,
    textOnAccent: "#ffffff",
  },
  surfaces: {
    bg: c.canvas,
    surface: c.surface1,
    // A card is cut from the same material as the chrome — on the darker plane it
    // reads as lifted without a second value.
    elevated: c.surface1,
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
  // Desaturated and lifted so they carry meaning on the warm near-black without
  // vibrating against it.
  semantic: {
    negative: "#e06c6c",
    warning: "#d6a750",
    info: c.accent,
    success: "#5fad65",
  },
  // Where each neutral sits, so the shell can rewrite them onto the live accent. The
  // hexes above are GENERATED from these steps at this theme's own accent — they are
  // what a cold boot paints, and the derivation reproduces them byte for byte while the
  // accent is untouched. Edit a step, regenerate the literal; they are one fact.
  neutralSteps: {
    surface: { l: 29.6, c: 0.01 },
    elevated: { l: 29.6, c: 0.01 },
    sunken: { l: 20.9, c: 0.015 },
    border: { l: 35.9, c: 0.0095 },
    borderSoft: { l: 42.3, c: 0.0107 },
  },
  // The primary CTA IS the accent here, not an inverting ink button: this
  // language spends colour on the one action that matters and keeps everything
  // else grey. Following `--color-accent` rather than a literal keeps the user's
  // accent pick on the button too.
  // One shade below the indicator accent, because the dark-scheme accents are
  // tuned to glow against near-black and carry white label text at only
  // 3.3–4.6:1. The light scheme needs no such step — see lyra-light.
  cta: {
    cta: "var(--color-accent-border)",
    ctaHover: "var(--color-accent-press)",
    ctaText: "var(--color-text-on-accent)",
  },
});
