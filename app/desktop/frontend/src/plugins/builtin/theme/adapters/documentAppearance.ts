import { colord } from "colord";
import type { StoreApi } from "zustand";
import type { ColorThemeId, VisualStyleId, VisualStyleMotion } from "@/lib/appearance";
import {
  publishMotionScale,
  publishScheme,
  publishTokens,
  publishVisualStyleMotion,
} from "@/lib/appearance";
import { densityCssVariables } from "@/lib/density";
import { uiTypeLadderCssVariables } from "@/lib/typography";
import { ACCENT, COLOR_THEME, VISUAL_STYLE } from "@/plugins/sdk/kernelPoints";
import { usePluginStore } from "@/plugins/sdk/registry";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import type { UiState } from "@/state/uiPreferences";
import { resolveThemeScheme } from "../application/themeScheme";
import { subscribeSystemScheme } from "./systemAppearance";

type UiEffectStore<T extends UiState> = Pick<StoreApi<T>, "getState" | "subscribe">;

function lightAccent(darkHex: string): string {
  const preset = lookupExtensionPoint(ACCENT).find((accent) => accent.dark === darkHex);
  return preset?.light ?? preset?.dark ?? colord(darkHex).darken(0.2).toHex();
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

function applyColorTheme(theme: ColorThemeId, accent: string, contrast: number): void {
  const root = document.documentElement;
  const scheme = resolveThemeScheme(theme);
  const spec = lookupExtensionByKey(COLOR_THEME, theme === "system" ? scheme : theme);

  root.classList.remove("theme-light", "theme-dark");
  root.classList.add(`theme-${scheme}`);
  appliedColorTokens = replaceTokens(appliedColorTokens, spec?.tokens ?? {});

  root.style.setProperty("--color-accent", scheme === "light" ? lightAccent(accent) : accent);
  appliedColorTokens.push("--color-accent");
  root.style.setProperty("--depth-step", `${(2 + (contrast / 100) * 8).toFixed(1)}%`);
  appliedColorTokens.push("--depth-step");

  publishScheme(scheme);
}

function applyVisualStyle(id: VisualStyleId): void {
  const root = document.documentElement;
  const spec =
    lookupExtensionByKey(VISUAL_STYLE, id) ?? lookupExtensionByKey(VISUAL_STYLE, "synara");
  const motionTokens = spec ? visualStyleMotionTokens(spec.motion) : {};
  appliedStyleTokens = replaceTokens(appliedStyleTokens, { ...spec?.tokens, ...motionTokens });
  if (spec) publishVisualStyleMotion(spec.motion);
  root.dataset.visualStyle = spec?.id ?? "synara";
  root.dataset.regionLayout = spec?.traits.regions ?? "floating-card";
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
    "ease-drawer": bezier(motion.easeDrawer),
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

  for (const [property, value] of Object.entries(uiTypeLadderCssVariables(fontSize))) {
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
  applyColorTheme(initial.theme, initial.accent, initial.contrast);
  applyVisualStyle(initial.visualStyle);
  publishTokens();
  applyFonts(initial.uiFont, initial.codeFont, initial.fontSize, initial.fontSmoothing);
  applyShape(initial.density, initial.radiusScale, initial.motionScale);

  const unsubscribeUi = store.subscribe((state, previous) => {
    if (
      state.theme !== previous.theme ||
      state.accent !== previous.accent ||
      state.contrast !== previous.contrast
    ) {
      applyColorTheme(state.theme, state.accent, state.contrast);
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

  const unsubscribePlugins = usePluginStore.subscribe((state, previous) => {
    if (state.extensions === previous.extensions) return;
    const current = store.getState();
    applyColorTheme(current.theme, current.accent, current.contrast);
    applyVisualStyle(current.visualStyle);
    publishTokens();
  });

  const unsubscribeScheme = subscribeSystemScheme(() => {
    const current = store.getState();
    if (current.theme !== "system") return;
    applyColorTheme(current.theme, current.accent, current.contrast);
    applyVisualStyle(current.visualStyle);
    publishTokens();
  });

  return () => {
    unsubscribeScheme();
    unsubscribePlugins();
    unsubscribeUi();
  };
}
