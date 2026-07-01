import type { ComposerDraftPort } from "../application/draft";
import { useComposerStore } from "./composerStore";

export const composerDraftPort: ComposerDraftPort = {
  replaceDraft(input) {
    const store = useComposerStore.getState();
    store.clear();
    store.setValue(input.text);
    if (input.images?.length) store.addImages(input.images);
  },
  focusDraftEnd(textLength) {
    const textarea = document.querySelector<HTMLTextAreaElement>(".composer-input");
    textarea?.focus();
    textarea?.setSelectionRange(textLength, textLength);
  },
};
