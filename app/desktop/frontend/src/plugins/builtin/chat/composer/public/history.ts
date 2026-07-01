import { useComposerStore } from "../adapters/composerStore";

export function useRecordComposerHistory(): (text: string) => void {
  return useComposerStore((state) => state.pushHistory);
}

export function recallPreviousComposerHistory(): boolean {
  return useComposerStore.getState().historyPrev();
}

export function recallNextComposerHistory(): boolean {
  return useComposerStore.getState().historyNext();
}
