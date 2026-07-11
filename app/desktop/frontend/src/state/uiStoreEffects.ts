import { colord } from "colord";
import type { StoreApi } from "zustand";
import { usePluginStore } from "@/plugins/sdk/registry";
import { ACCENT, THEME } from "@/plugins/sdk/kernelPoints";
import { lookupExtensionByKey, lookupExtensionPoint } from "@/plugins/sdk/selectors/extensions";
import type { Theme, UiState } from "./uiPreferences";

type UiEffectStore<T extends UiState> = Pick<StoreApi<T>, "getState" | "subscribe">;

function lightAccent(darkHex: string): string {
  const preset = lookupExtensionPoint(ACCENT).find((accent) => accent.dark === darkHex);
  return preset?.light ?? preset?.dark ?? colord(darkHex).darken(0.2).toHex();
}

function prefersDark(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof window.matchMedia === "function" &&
    window.matchMedia("(prefers-color-scheme: dark)").matches
  );
}

export function resolveThemeId(theme: Theme): Theme {
  return theme === "system" ? (prefersDark() ? "dark" : "light") : theme;
}

let appliedTokenNames: string[] = [];

function applyTheme(theme: Theme, accent: string, contrast: number): void {
  const root = document.documentElement;
  const resolved = resolveThemeId(theme);
  const spec = lookupExtensionByKey(THEME, resolved);
  const scheme = spec?.scheme ?? (resolved === "light" ? "light" : "dark");

  root.classList.remove("theme-light", "theme-dark");
  root.classList.add(`theme-${scheme}`);

  for (const name of appliedTokenNames) root.style.removeProperty(name);
  appliedTokenNames = [];

  for (const [name, value] of Object.entries(spec?.tokens ?? {})) {
    if (name === "color-surface-2" || name === "color-surface-3" || name === "color-surface-4") {
      continue;
    }
    const property = `--${name}`;
    root.style.setProperty(property, value);
    appliedTokenNames.push(property);
  }

  root.style.setProperty("--color-accent", scheme === "light" ? lightAccent(accent) : accent);
  appliedTokenNames.push("--color-accent");

  root.style.setProperty("--depth-step", `${(2 + (contrast / 100) * 8).toFixed(1)}%`);
  appliedTokenNames.push("--depth-step");
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

  root.style.fontSize = fontSize ? `${fontSize}px` : "";
}

function applyShape(radiusScale: number, motionScale: number): void {
  const root = document.documentElement;
  root.style.setProperty("--radius-scale", String(radiusScale));
  root.style.setProperty("--motion-scale", String(motionScale));
  if (motionScale === 0) root.setAttribute("data-motion", "off");
  else root.removeAttribute("data-motion");
}

export function installUiStoreEffects<T extends UiState>(store: UiEffectStore<T>): () => void {
  const initial = store.getState();
  applyTheme(initial.theme, initial.accent, initial.contrast);
  applyFonts(initial.uiFont, initial.codeFont, initial.fontSize, initial.fontSmoothing);
  applyShape(initial.radiusScale, initial.motionScale);

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
    if (state.radiusScale !== previous.radiusScale || state.motionScale !== previous.motionScale) {
      applyShape(state.radiusScale, state.motionScale);
    }
  });

  const unsubscribePlugins = usePluginStore.subscribe((state, previous) => {
    if (state.extensions === previous.extensions) return;
    const { theme, accent, contrast } = store.getState();
    applyTheme(theme, accent, contrast);
  });

  let unsubscribeScheme = () => {};
  if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const onSchemeChange = () => {
      const { theme, accent, contrast } = store.getState();
      if (theme === "system") applyTheme(theme, accent, contrast);
    };
    media.addEventListener("change", onSchemeChange);
    unsubscribeScheme = () => media.removeEventListener("change", onSchemeChange);
  }

  return () => {
    unsubscribeScheme();
    unsubscribePlugins();
    unsubscribeUi();
  };
}
