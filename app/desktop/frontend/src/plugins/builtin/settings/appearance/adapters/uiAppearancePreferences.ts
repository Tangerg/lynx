import { useUiStore } from "@/state/uiStore";
import { configureAppearancePreferencesPort } from "../application/ports/preferences";

export function installAppearancePreferencesPort(): () => void {
  return configureAppearancePreferencesPort({
    useTheme: () => useUiStore((state) => state.theme),
    useSetTheme: () => useUiStore((state) => state.setTheme),
    useAccent: () => useUiStore((state) => state.accent),
    useSetAccent: () => useUiStore((state) => state.setAccent),
    useCustomTheme: () => useUiStore((state) => state.customTheme),
    useSetCustomTheme: () => useUiStore((state) => state.setCustomTheme),
    useContrast: () => useUiStore((state) => state.contrast),
    useSetContrast: () => useUiStore((state) => state.setContrast),
    useUiFont: () => useUiStore((state) => state.uiFont),
    useCodeFont: () => useUiStore((state) => state.codeFont),
    useFontSize: () => useUiStore((state) => state.fontSize),
    useFontSmoothing: () => useUiStore((state) => state.fontSmoothing),
    useSetUiFont: () => useUiStore((state) => state.setUiFont),
    useSetCodeFont: () => useUiStore((state) => state.setCodeFont),
    useSetFontSize: () => useUiStore((state) => state.setFontSize),
    useSetFontSmoothing: () => useUiStore((state) => state.setFontSmoothing),
    useRadiusScale: () => useUiStore((state) => state.radiusScale),
    useMotionScale: () => useUiStore((state) => state.motionScale),
    useSetRadiusScale: () => useUiStore((state) => state.setRadiusScale),
    useSetMotionScale: () => useUiStore((state) => state.setMotionScale),
  });
}
