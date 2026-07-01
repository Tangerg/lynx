import { useComposerStore } from "../adapters/composerStore";

export function recallPreviousComposerHistory(): boolean {
  return useComposerStore.getState().historyPrev();
}

export function recallNextComposerHistory(): boolean {
  return useComposerStore.getState().historyNext();
}
