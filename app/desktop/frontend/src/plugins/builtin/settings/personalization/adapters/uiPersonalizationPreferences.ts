import { useUiStore } from "@/state/uiStore";
import { configurePersonalizationPreferencesPort } from "../application/ports/preferences";

export function installPersonalizationPreferencesPort(): () => void {
  return configurePersonalizationPreferencesPort({
    useCompletionSound: () => useUiStore((state) => state.completionSound),
    useSetCompletionSound: () => useUiStore((state) => state.setCompletionSound),
    useStreamReveal: () => useUiStore((state) => state.streamReveal),
    useSetStreamReveal: () => useUiStore((state) => state.setStreamReveal),
  });
}
