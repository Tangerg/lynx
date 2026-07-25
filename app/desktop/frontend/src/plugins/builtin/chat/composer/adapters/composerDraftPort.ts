import type { ComposerDraftPort } from "../application/draft";
import { focusComposer } from "../application/focus";
import { useComposerStore } from "./composerStore";

export const composerDraftPort: ComposerDraftPort = {
  replaceDraft(input) {
    const store = useComposerStore.getState();
    store.clear();
    store.setValue(input.text);
    if (input.images?.length) store.addImages(input.images);
  },
  focusDraftEnd(textLength) {
    focusComposer(textLength);
  },
};
