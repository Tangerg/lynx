import { disposeOnHmr } from "@/lib/hmr";
import {
  getAgentSessionLifecycleSnapshot,
  subscribeAgentSessionLifecycle,
} from "@/plugins/builtin/agent/public/session";
import { replaceDraft } from "../application/draft";
import { configureComposerStatePort } from "../application/ports/state";
import { composerDraftPort } from "./composerDraftPort";
import { useComposerStore } from "./composerStore";

let stopSessionSync: (() => void) | null = null;

export function installComposerStatePorts(): void {
  configureComposerStatePort({
    useText: () => useComposerStore((state) => state.value),
    useSetText: () => useComposerStore((state) => state.setValue),
    useClearDraft: () => useComposerStore((state) => state.clear),
    getText: () => useComposerStore.getState().value,
    replaceDraft: (input) => replaceDraft(input, composerDraftPort),
    useImages: () => useComposerStore((state) => state.images),
    usePastes: () => useComposerStore((state) => state.pastes),
    useAddImageFiles: () => useComposerStore((state) => state.addImageFiles),
    useRemoveImage: () => useComposerStore((state) => state.removeImage),
    useAddPaste: () => useComposerStore((state) => state.addPaste),
    useRemovePaste: () => useComposerStore((state) => state.removePaste),
    useRecordHistory: () => useComposerStore((state) => state.pushHistory),
    recallPreviousHistory: () => useComposerStore.getState().historyPrev(),
    recallNextHistory: () => useComposerStore.getState().historyNext(),
    getModelPreference: () => {
      const { provider, model } = useComposerStore.getState();
      return { provider, model };
    },
    useModelPreference: () => {
      const provider = useComposerStore((state) => state.provider);
      const model = useComposerStore((state) => state.model);
      return { provider, model };
    },
    useSetModelPreference: () => useComposerStore((state) => state.setModel),
  });

  installComposerSessionSync();
}

function installComposerSessionSync(): void {
  stopSessionSync?.();
  stopSessionSync = subscribeAgentSessionLifecycle(({ activeSessionId, openSessionIds }) => {
    const composer = useComposerStore.getState();
    composer.loadSession(activeSessionId);
    composer.pruneDrafts(new Set(openSessionIds));
  });

  const initial = getAgentSessionLifecycleSnapshot();
  useComposerStore.getState().loadSession(initial.activeSessionId);
  useComposerStore.getState().pruneDrafts(new Set(initial.openSessionIds));
}

disposeOnHmr(() => {
  stopSessionSync?.();
  stopSessionSync = null;
});
