import { colord } from "colord";
import type { StoreApi } from "zustand";
import type {
  AccentTint,
  ColorThemeId,
  Scheme,
  VisualStyleId,
  VisualStyleMotion,
} from "@/lib/appearance";
import {
  publishMotionScale,
  publishScheme,
  publishTokens,
  publishVisualStyleMotion,
} from "@/lib/appearance";
import { densityCssVariables } from "@/lib/density";
import { iconScaleCssVariables } from "@/lib/iconScale";
import { uiTypeLadderCssVariables } from "@/lib/typography";
import type { ColorThemeSpec, NeutralStep } from "@/plugins/sdk";
import { ACCENT, COLOR_THEME, VISUAL_STYLE } from "@/plugins/sdk/kernelPoints";
import { subscribeContributions } from "@/plugins/sdk";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import type { UiState } from "@/state/uiPreferences";
import { accentTintedNeutral } from "../kit/accentTint";
import { resolveThemeScheme } from "../application/themeScheme";
import { subscribeSystemScheme } from "./systemAppearance";

type UiEffectStore<T extends UiState> = Pick<StoreApi<T>, "getState" | "subscribe">;

function lightAccent(darkHex: string): string {
  const preset = lookupExtensionPoint(ACCENT).find((accent) => accent.dark === darkHex);
  return preset?.light ?? preset?.dark ?? colord(darkHex).darken(0.2).toHex();
}

/**
 * Contrast preference → the surface-ladder step every derived rung reads.
 *
 * Doubled on dark, because equal ink percentages do not buy equal separation.
 * Mixing 4% of a near-white ink into a near-black surface moves it roughly a
 * third as far in perceived lightness as mixing 4% of a near-black ink into a
 * near-white one — so at the contrast setting that reads right on light, every
 * dark scheme's regions, chips and row states collapsed into one flat value.
 */
function depthStep(scheme: Scheme, contrast: number): string {
  const step = (2 + (contrast / 100) * 8) * (scheme === "dark" ? 2 : 1);
  return `${step.toFixed(1)}%`;
}

function replaceTokens(previous: string[], tokens: Record<string, string>): string[] {
  const root = document.documentElement;
  for (const property of previous) root.style.removeProperty(property);
  const next: string[] = [];
  for (const [name, value] of Object.entries(tokens)) {
    const property = `--${name}`;
    root.style.setProperty(property, value);
    next.push(property);
  }
  return next;
}

let appliedColorTokens: string[] = [];
let appliedStyleTokens: string[] = [];

/**
 * The neutral family, rewritten onto the live accent for a theme that opted in.
 *
 * A theme's `surfaces` / `borders` literals are the same family at the DEFAULT accent —
 * they are what the pre-paint script and the stylesheet mirror carry — so this returns
 * an override rather than the whole map, and returns nothing at all for a palette theme
 * (Solarized's base3 is Solarized, not a tint of whatever is selected).
 */
function neutralOverride(
  spec: ColorThemeSpec | undefined,
  liveAccent: string,
  tint: AccentTint,
): Record<string, string> {
  const steps = spec?.neutralSteps;
  // The theme's own accent is the reference the derivation is relative to, so an
  // untouched accent reproduces its literals byte for byte. A theme whose accent is not
  // a plain hex (a palette pointing at a var) opts out by having nothing to measure
  // against.
  const reference = spec?.tokens?.["color-accent"];
  if (!steps || !reference || !/^#[\da-f]{6}$/i.test(reference)) return {};
  const tinted = (step: NeutralStep) => accentTintedNeutral(liveAccent, reference, step, tint);
  return {
    "color-surface": tinted(steps.surface),
    "color-elevated": tinted(steps.elevated),
    "color-sunken": tinted(steps.sunken),
    "color-border": tinted(steps.border),
    "color-border-soft": tinted(steps.borderSoft),
  };
}

function applyColorTheme(
  theme: ColorThemeId,
  accent: string,
  contrast: number,
  accentTint: AccentTint,
): void {
  const root = document.documentElement;
  const scheme = resolveThemeScheme(theme);
  const spec = lookupExtensionByKey(COLOR_THEME, theme === "system" ? scheme : theme);

  root.classList.remove("theme-light", "theme-dark");
  root.classList.add(`theme-${scheme}`);

  // The accent's hover and press shades follow the LIVE accent, not the theme's
  // declared one, keeping every interaction state on the selected hue.
  const liveAccent = scheme === "light" ? lightAccent(accent) : accent;
  appliedColorTokens = replaceTokens(appliedColorTokens, {
    ...spec?.tokens,
    ...neutralOverride(spec, liveAccent, accentTint),
  });

  root.style.setProperty("--color-accent", liveAccent);
  root.style.setProperty("--color-accent-border", colord(liveAccent).darken(0.08).toHex());
  root.style.setProperty("--color-accent-press", colord(liveAccent).darken(0.16).toHex());
  root.style.setProperty("--depth-step", depthStep(scheme, contrast));
  appliedColorTokens.push(
    "--color-accent",
    "--color-accent-border",
    "--color-accent-press",
    "--depth-step",
  );

  publishScheme(scheme);
}

function applyVisualStyle(id: VisualStyleId): void {
  const root = document.documentElement;
  const spec =
    lookupExtensionByKey(VISUAL_STYLE, id) ?? lookupExtensionByKey(VISUAL_STYLE, "scopeapp");
  const motionTokens = spec ? visualStyleMotionTokens(spec.motion) : {};
  appliedStyleTokens = replaceTokens(appliedStyleTokens, { ...spec?.tokens, ...motionTokens });
  if (spec) publishVisualStyleMotion(spec.motion);
  root.dataset.visualStyle = spec?.id ?? "scopeapp";
  root.dataset.regionLayout = spec?.traits.regions ?? "tonal-columns";
  root.dataset.controlTreatment = spec?.traits.controls ?? "quiet";
}

function visualStyleMotionTokens(motion: VisualStyleMotion): Record<string, string> {
  const bezier = (value: readonly [number, number, number, number]) =>
    `cubic-bezier(${value.join(", ")})`;
  return {
    "dur-instant-base": `${motion.instantMs}ms`,
    "dur-fast-base": `${motion.fastMs}ms`,
    "dur-med-base": `${motion.mediumMs}ms`,
    "dur-disclosure-base": `${motion.disclosureMs}ms`,
    "dur-slow-base": `${motion.slowMs}ms`,
    "dur-drawer-base": `${motion.drawerMs}ms`,
    "ease-out": bezier(motion.easeOut),
    "ease-in-out": bezier(motion.easeInOut),
    "ease-emphasized": bezier(motion.easeEmphasized),
    "ease-drawer": `linear(${motion.drawerProgress.join(", ")})`,
    "press-scale": String(motion.pressScale),
  };
}

function applyFonts(
  uiFont: string,
  codeFont: string,
  fontSize: number | null,
  fontSmoothing: boolean,
): void {
  const root = document.documentElement;
  root.style.setProperty("-webkit-font-smoothing", fontSmoothing ? "antialiased" : "auto");
  root.style.setProperty("-moz-osx-font-smoothing", fontSmoothing ? "grayscale" : "auto");

  if (uiFont) {
    root.style.setProperty(
      "--font-sans",
      `"${uiFont}", -apple-system, system-ui, "PingFang SC", sans-serif`,
    );
  } else {
    root.style.removeProperty("--font-sans");
  }

  if (codeFont) {
    root.style.setProperty(
      "--font-mono",
      `"${codeFont}", ui-monospace, "SF Mono", Menlo, monospace`,
    );
  } else {
    root.style.removeProperty("--font-mono");
  }

  // The icon ladder rides the same base: a glyph beside a label must grow with it,
  // and its stroke is derived from the size it lands on.
  for (const [property, value] of Object.entries({
    ...uiTypeLadderCssVariables(fontSize),
    ...iconScaleCssVariables(fontSize),
  })) {
    root.style.setProperty(property, value);
  }
}

function applyShape(density: string, radiusScale: number, motionScale: number): void {
  const root = document.documentElement;
  for (const [property, value] of Object.entries(densityCssVariables(density))) {
    root.style.setProperty(property, value);
  }
  root.style.setProperty("--radius-scale", String(radiusScale));
  root.style.setProperty("--motion-scale", String(motionScale));
  publishMotionScale(motionScale);
  if (motionScale === 0) root.setAttribute("data-motion", "off");
  else root.removeAttribute("data-motion");
}

export function installDocumentAppearance<T extends UiState>(store: UiEffectStore<T>): () => void {
  const initial = store.getState();
  applyColorTheme(initial.theme, initial.accent, initial.contrast, initial.accentTint);
  applyVisualStyle(initial.visualStyle);
  publishTokens();
  applyFonts(initial.uiFont, initial.codeFont, initial.fontSize, initial.fontSmoothing);
  applyShape(initial.density, initial.radiusScale, initial.motionScale);

  const unsubscribeUi = store.subscribe((state, previous) => {
    if (
      state.theme !== previous.theme ||
      state.accent !== previous.accent ||
      state.contrast !== previous.contrast ||
      state.accentTint !== previous.accentTint
    ) {
      applyColorTheme(state.theme, state.accent, state.contrast, state.accentTint);
      applyVisualStyle(state.visualStyle);
      publishTokens();
    } else if (state.visualStyle !== previous.visualStyle) {
      applyVisualStyle(state.visualStyle);
      publishTokens();
    }
    if (
      state.uiFont !== previous.uiFont ||
      state.codeFont !== previous.codeFont ||
      state.fontSize !== previous.fontSize ||
      state.fontSmoothing !== previous.fontSmoothing
    ) {
      applyFonts(state.uiFont, state.codeFont, state.fontSize, state.fontSmoothing);
    }
    if (
      state.density !== previous.density ||
      state.radiusScale !== previous.radiusScale ||
      state.motionScale !== previous.motionScale
    ) {
      applyShape(state.density, state.radiusScale, state.motionScale);
    }
  });

  const unsubscribePlugins = subscribeContributions(() => {
    const current = store.getState();
    applyColorTheme(current.theme, current.accent, current.contrast, current.accentTint);
    applyVisualStyle(current.visualStyle);
    publishTokens();
  });

  const unsubscribeScheme = subscribeSystemScheme(() => {
    const current = store.getState();
    if (current.theme !== "system") return;
    applyColorTheme(current.theme, current.accent, current.contrast, current.accentTint);
    applyVisualStyle(current.visualStyle);
    publishTokens();
  });

  return () => {
    unsubscribeScheme();
    unsubscribePlugins();
    unsubscribeUi();
  };
}
