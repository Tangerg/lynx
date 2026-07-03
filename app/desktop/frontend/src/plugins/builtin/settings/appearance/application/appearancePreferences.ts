import { resolveThemeScheme } from "@/plugins/builtin/theme/public/scheme";
import { appearancePreferences, type CustomTheme, type Theme } from "./ports/preferences";

export function useThemePreference() {
  return {
    theme: appearancePreferences().useTheme(),
    setTheme: appearancePreferences().useSetTheme(),
  };
}

export function useAccentPreference() {
  const theme = appearancePreferences().useTheme();
  return {
    accent: appearancePreferences().useAccent(),
    setAccent: appearancePreferences().useSetAccent(),
    scheme: resolveThemeScheme(theme),
  };
}

export function useCustomThemePreference(): {
  theme: Theme;
  customTheme: CustomTheme;
  setCustomTheme: (patch: Partial<CustomTheme>) => void;
} {
  return {
    theme: appearancePreferences().useTheme(),
    customTheme: appearancePreferences().useCustomTheme(),
    setCustomTheme: appearancePreferences().useSetCustomTheme(),
  };
}

export function useContrastPreference() {
  return {
    contrast: appearancePreferences().useContrast(),
    setContrast: appearancePreferences().useSetContrast(),
  };
}

export function useFontPreferences() {
  return {
    uiFont: appearancePreferences().useUiFont(),
    codeFont: appearancePreferences().useCodeFont(),
    fontSize: appearancePreferences().useFontSize(),
    fontSmoothing: appearancePreferences().useFontSmoothing(),
    setUiFont: appearancePreferences().useSetUiFont(),
    setCodeFont: appearancePreferences().useSetCodeFont(),
    setFontSize: appearancePreferences().useSetFontSize(),
    setFontSmoothing: appearancePreferences().useSetFontSmoothing(),
  };
}

export function useShapeMotionPreferences() {
  return {
    radiusScale: appearancePreferences().useRadiusScale(),
    motionScale: appearancePreferences().useMotionScale(),
    setRadiusScale: appearancePreferences().useSetRadiusScale(),
    setMotionScale: appearancePreferences().useSetMotionScale(),
  };
}
