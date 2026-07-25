// Paints the user's appearance preferences onto the document: theme tokens and
// scheme class, accent, contrast depth, fonts, density, radius and motion.
//
// This lives in the theme context, not beside the store it reads. The store's job
// is to hold the preference; turning "contrast: 40" into `--depth-step: 5.2%` is
// presentation, and *which themes exist* is this context's business — it owns the
// THEME and ACCENT extension points and the token vocabulary in globals.css. It
// sat in `state/` for a while, which meant the store layer reached up into the
// plugin registry and knew the names of custom properties.

import { colord } from "colord";
import type { StoreApi } from "zustand";
import { usePluginStore } from "@/plugins/sdk/registry";
import { ACCENT, THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import { resolveThemeScheme } from "../application/themeScheme";
import { subscribeSystemScheme } from "./systemAppearance";
import { publishMotionScale, publishScheme } from "@/lib/appearance";
import { densityCssVariables } from "@/lib/density";
import { uiTypeLadderCssVariables } from "@/lib/typography";
import type { Theme, UiState } from "@/state/uiPreferences";

type UiEffectStore<T extends UiState> = Pick<StoreApi<T>, "getState" | "subscribe">;

function lightAccent(darkHex: string): string {
  const preset = lookupExtensionPoint(ACCENT).find((accent) => accent.dark === darkHex);
  return preset?.light ?? preset?.dark ?? colord(darkHex).darken(0.2).toHex();
}

let appliedTokenNames: string[] = [];

function applyTheme(theme: Theme, accent: string, contrast: number): void {
  const root = document.documentElement;
  const scheme = resolveThemeScheme(theme);
  const spec = lookupExtensionByKey(THEME, theme === "system" ? scheme : theme);

  root.classList.remove("theme-light", "theme-dark");
  root.classList.add(`theme-${scheme}`);

  for (const name of appliedTokenNames) root.style.removeProperty(name);
  appliedTokenNames = [];

  for (const [name, value] of Object.entries(spec?.tokens ?? {})) {
    const property = `--${name}`;
    root.style.setProperty(property, value);
    appliedTokenNames.push(property);
  }

  root.style.setProperty("--color-accent", scheme === "light" ? lightAccent(accent) : accent);
  appliedTokenNames.push("--color-accent");

  root.style.setProperty("--depth-step", `${(2 + (contrast / 100) * 8).toFixed(1)}%`);
  appliedTokenNames.push("--depth-step");

  // Leaf code (the Shiki preset) can't read the store or the registry — the
  // scheme reaches it from here, where it just became true.
  publishScheme(scheme);
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

  // The type base drives the derived `--fs-*` ladder, NOT the root font-size.
  // Scaling `<html>` would move every rem-based padding, gap and width along with
  // the text, so fixed chrome geometry (header height, row height) could not hold.
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

export function installDocumentTheme<T extends UiState>(store: UiEffectStore<T>): () => void {
  const initial = store.getState();
  applyTheme(initial.theme, initial.accent, initial.contrast);
  applyFonts(initial.uiFont, initial.codeFont, initial.fontSize, initial.fontSmoothing);
  applyShape(initial.density, initial.radiusScale, initial.motionScale);

  const unsubscribeUi = store.subscribe((state, previous) => {
    if (
      state.theme !== previous.theme ||
      state.accent !== previous.accent ||
      state.contrast !== previous.contrast
    ) {
      applyTheme(state.theme, state.accent, state.contrast);
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
    const { theme, accent, contrast } = store.getState();
    applyTheme(theme, accent, contrast);
  });

  const unsubscribeScheme = subscribeSystemScheme(() => {
    const { theme, accent, contrast } = store.getState();
    if (theme === "system") applyTheme(theme, accent, contrast);
  });

  return () => {
    unsubscribeScheme();
    unsubscribePlugins();
    unsubscribeUi();
  };
}
