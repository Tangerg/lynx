// Deriving the neutral family's hue from the live accent.
//
// The family sits on the accent's hue at a chroma chosen per surface by area (see
// themes/scopeapp-light.ts). Expressing that in CSS as
// `oklch(from var(--color-accent) <L> <C> h)` looks like the whole answer and is not:
// a GREY ACCENT HAS NO HUE, and CSS reads the powerless channel as 0, which is RED —
// so choosing pure black painted every surface pink. A colour function cannot branch,
// so the derivation lives here.
//
// The rule is Material's, from the Fidelity and Content schemes: a neutral palette's
// chroma is PROPORTIONAL TO THE SOURCE COLOUR'S OWN (`sourceColorHct.chroma / 8.0` in
// material-color-utilities). That single choice does three things a fixed number
// cannot — a grey accent yields grey surfaces with no special case, a muted accent
// tints less than a vivid one, and the amount is always relative to how much colour
// the user actually asked for.
//
// The other two systems worth reading agree on the shape and differ on the details:
//
//   Material  neutral chroma is a fixed low number per SCHEME VARIANT — TonalSpot 6,
//             Vibrant 10, Neutral 2 — or `sourceChroma / 8` for Fidelity/Content.
//             Expressive rotates the neutral hue +15°. Gamut safety comes from HCT
//             solving for the closest achievable chroma at each tone.
//   Ant       branches on hue, but to compensate DRIFT, not to damp colour: inside
//             60–240° a lighter step rotates -2° and a darker one +2°, and outside
//             that range the direction flips. Greys (h=0,s=0) are left alone.
//   Apple     does not derive surfaces at all. System colours are curated per
//             appearance, contrast and vibrancy, and the accent tints CONTROLS.
//
// Notably none of the three scales chroma by hue, so neither do we: a violet accent
// gives violet-tinted surfaces, which under Material's model is the point of a
// personalised palette. Whether that is too much is a taste axis, and Material settles
// taste axes with a variant — hence `AccentTint` below rather than a constant someone
// has to argue with.

import type { AccentTint } from "@/lib/appearance";
import { DEFAULT_ACCENT_TINT } from "@/lib/appearance";
import type { NeutralStep } from "@/plugins/sdk";

/** OKLCH, degrees for hue. */
export interface Oklch {
  l: number;
  c: number;
  h: number;
}

const srgbToLinear = (value: number) =>
  value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
const linearToSrgb = (value: number) =>
  value <= 0.0031308 ? 12.92 * value : 1.055 * value ** (1 / 2.4) - 0.055;

export function hexToOklch(hex: string): Oklch {
  const value = Number.parseInt(hex.replace("#", ""), 16);
  const [r, g, b] = [(value >> 16) & 255, (value >> 8) & 255, value & 255].map((channel) =>
    srgbToLinear(channel / 255),
  ) as [number, number, number];
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  const lightness = 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s;
  const a = 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s;
  const bAxis = 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s;
  const hue = (Math.atan2(bAxis, a) * 180) / Math.PI;
  return { l: lightness * 100, c: Math.hypot(a, bAxis), h: hue < 0 ? hue + 360 : hue };
}

function toLinearRgb({ l, c, h }: Oklch): [number, number, number] {
  const a = c * Math.cos((h * Math.PI) / 180);
  const b = c * Math.sin((h * Math.PI) / 180);
  const lightness = l / 100;
  const lp = (lightness + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const mp = (lightness - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const sp = (lightness - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    4.0767416621 * lp - 3.3077115913 * mp + 0.2309699292 * sp,
    -1.2684380046 * lp + 2.6097574011 * mp - 0.3413193965 * sp,
    -0.0041960863 * lp - 0.7034186147 * mp + 1.707614701 * sp,
  ];
}

const IN_GAMUT_EPSILON = 1 / 512;

function inSrgbGamut(colour: Oklch): boolean {
  return toLinearRgb(colour).every(
    (channel) => channel >= -IN_GAMUT_EPSILON && channel <= 1 + IN_GAMUT_EPSILON,
  );
}

/**
 * OKLCH → `#rrggbb`, giving up CHROMA rather than hue when a request leaves sRGB.
 *
 * Clamping the channels instead — what a naive conversion does, and what the browser
 * does at paint time — lands the colour on whichever channel saturated first and takes
 * the hue with it. Walking the chroma down keeps the hue and lightness that were asked
 * for and surrenders only the part the display cannot show. This is what HCT does
 * inside `Hct.from`, by the same binary search.
 */
export function oklchToHex(colour: Oklch): string {
  let fitted = colour;
  if (!inSrgbGamut(fitted)) {
    let low = 0;
    let high = colour.c;
    for (let step = 0; step < 16; step += 1) {
      const mid = (low + high) / 2;
      if (inSrgbGamut({ ...colour, c: mid })) low = mid;
      else high = mid;
    }
    fitted = { ...colour, c: low };
  }
  return `#${toLinearRgb(fitted)
    .map((channel) => Math.max(0, Math.min(255, Math.round(linearToSrgb(channel) * 255))))
    .map((channel) => channel.toString(16).padStart(2, "0"))
    .join("")}`;
}

const TINT_SCALE: Record<AccentTint, number> = { off: 0, soft: 0.5, standard: 1 };

/** A neon accent may not run away with the family. sRGB tops out near C 0.32, which is
 *  1.6-1.7× a typical reference. */
const MAX_CHROMA_FACTOR = 1.5;

/**
 * Multiplier applied to every surface's declared chroma budget.
 *
 * The reference is the accent THE THEME ITSELF DECLARES, not a module constant.
 * Dividing by the theme's own accent — rather than by Material's constant 8, which is
 * in HCT units — is what makes an untouched accent reproduce that theme's declared
 * literals exactly, so the derivation is a no-op until someone picks something else. A
 * single constant could not do that: the two schemes ship different accents (#2b5fd0
 * light, #3574f0 dark), so whichever one the constant matched, the other drifted.
 */
export function neutralChromaFactor(
  accentChroma: number,
  referenceChroma: number,
  tint: AccentTint,
): number {
  if (referenceChroma <= 0) return 0;
  return Math.min(MAX_CHROMA_FACTOR, accentChroma / referenceChroma) * TINT_SCALE[tint];
}

/**
 * One neutral step, rewritten onto the accent's hue.
 *
 * Returns a hex rather than an `oklch(…)` string so a caller can hand it straight to a
 * token map, and so a test and devtools both read the value that will actually paint.
 *
 * One step at a time rather than a generic over the whole family: an interface has no
 * index signature, so a `Record<string, NeutralStep>` constraint rejects the very type
 * the theme spec publishes, and working around that costs more than the loop it saves.
 */
export function accentTintedNeutral(
  accentHex: string,
  referenceAccentHex: string,
  step: NeutralStep,
  tint: AccentTint = DEFAULT_ACCENT_TINT,
): string {
  const accent = hexToOklch(accentHex);
  const factor = neutralChromaFactor(accent.c, hexToOklch(referenceAccentHex).c, tint);
  // Hue only matters when there is chroma to carry it; at zero it would be an arbitrary
  // number in the output, and 0 reads as red to anyone inspecting it.
  return oklchToHex({ l: step.l, c: step.c * factor, h: factor === 0 ? 0 : accent.h });
}
