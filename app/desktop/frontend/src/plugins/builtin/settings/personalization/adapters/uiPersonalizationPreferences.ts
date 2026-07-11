import { useUiStore } from "@/state/uiStore";
import { configurePersonalizationPreferencesPort } from "../application/ports/preferences";

export function installPersonalizationPreferencesPort(): () => void {
  return configurePersonalizationPreferencesPort({
    useMessageStyle: () => useUiStore((state) => state.messageStyle),
    useSetMessageStyle: () => useUiStore((state) => state.setMessageStyle),
    useCompletionSound: () => useUiStore((state) => state.completionSound),
    useSetCompletionSound: () => useUiStore((state) => state.setCompletionSound),
    useStreamReveal: () => useUiStore((state) => state.streamReveal),
    useSetStreamReveal: () => useUiStore((state) => state.setStreamReveal),
  });
}
