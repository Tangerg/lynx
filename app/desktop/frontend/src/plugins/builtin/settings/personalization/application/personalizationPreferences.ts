import { useUiStore } from "@/state/uiStore";

export function useMessageStylePreference() {
  return {
    messageStyle: useUiStore((state) => state.messageStyle),
    setMessageStyle: useUiStore((state) => state.setMessageStyle),
  };
}

export function useCompletionSoundPreference() {
  return {
    completionSound: useUiStore((state) => state.completionSound),
    setCompletionSound: useUiStore((state) => state.setCompletionSound),
  };
}

export function useStreamRevealPreference() {
  return {
    streamReveal: useUiStore((state) => state.streamReveal),
    setStreamReveal: useUiStore((state) => state.setStreamReveal),
  };
}
