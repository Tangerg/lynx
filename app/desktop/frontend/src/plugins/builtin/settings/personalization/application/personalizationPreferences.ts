import { personalizationPreferences } from "./ports/preferences";

export function useMessageStylePreference() {
  return {
    messageStyle: personalizationPreferences().useMessageStyle(),
    setMessageStyle: personalizationPreferences().useSetMessageStyle(),
  };
}

export function useCompletionSoundPreference() {
  return {
    completionSound: personalizationPreferences().useCompletionSound(),
    setCompletionSound: personalizationPreferences().useSetCompletionSound(),
  };
}

export function useStreamRevealPreference() {
  return {
    streamReveal: personalizationPreferences().useStreamReveal(),
    setStreamReveal: personalizationPreferences().useSetStreamReveal(),
  };
}
