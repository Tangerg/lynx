import type { CustomTheme, Theme } from "@/state/uiStore";
import { resolveScheme } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";

export function useThemePreference() {
  return {
    theme: useUiStore((state) => state.theme),
    setTheme: useUiStore((state) => state.setTheme),
  };
}

export function useAccentPreference() {
  const theme = useUiStore((state) => state.theme);
  return {
    accent: useUiStore((state) => state.accent),
    setAccent: useUiStore((state) => state.setAccent),
    scheme: resolveScheme(theme),
  };
}

export function useCustomThemePreference(): {
  theme: Theme;
  customTheme: CustomTheme;
  setCustomTheme: (patch: Partial<CustomTheme>) => void;
} {
  return {
    theme: useUiStore((state) => state.theme),
    customTheme: useUiStore((state) => state.customTheme),
    setCustomTheme: useUiStore((state) => state.setCustomTheme),
  };
}

export function useContrastPreference() {
  return {
    contrast: useUiStore((state) => state.contrast),
    setContrast: useUiStore((state) => state.setContrast),
  };
}

export function useFontPreferences() {
  return {
    uiFont: useUiStore((state) => state.uiFont),
    codeFont: useUiStore((state) => state.codeFont),
    fontSize: useUiStore((state) => state.fontSize),
    fontSmoothing: useUiStore((state) => state.fontSmoothing),
    setUiFont: useUiStore((state) => state.setUiFont),
    setCodeFont: useUiStore((state) => state.setCodeFont),
    setFontSize: useUiStore((state) => state.setFontSize),
    setFontSmoothing: useUiStore((state) => state.setFontSmoothing),
  };
}

export function useShapeMotionPreferences() {
  return {
    radiusScale: useUiStore((state) => state.radiusScale),
    motionScale: useUiStore((state) => state.motionScale),
    setRadiusScale: useUiStore((state) => state.setRadiusScale),
    setMotionScale: useUiStore((state) => state.setMotionScale),
  };
}
