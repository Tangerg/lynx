import { useUiStore } from "@/state/uiStore";
import { configureThemePreferencePort } from "../application/ports/themePreference";

export function installThemePreferencePort(): () => void {
  return configureThemePreferencePort({
    activeTheme: () => useUiStore.getState().theme,
    setTheme: (id) => useUiStore.getState().setTheme(id),
  });
}
