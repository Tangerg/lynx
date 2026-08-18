import { disposeOnHmr } from "@/lib/hmr";
import type { AgentSessionPorts } from "@/plugins/builtin/agent/public/ports";
import { replaceDraft } from "../application/draft";
import { configureComposerStatePort } from "../application/ports/state";
import { composerDraftPort } from "./composerDraftPort";
import { useComposerStore } from "./composerStore";

let stopSessionSync: (() => void) | null = null;

export function installComposerStatePorts(sessions: AgentSessionPorts): () => void {
  const disposePort = configureComposerStatePort({
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
  const disposeSessionSync = installComposerSessionSync(sessions);
  return () => {
    disposeSessionSync();
    disposePort();
  };
}

function installComposerSessionSync(sessions: AgentSessionPorts): () => void {
  stopSessionSync?.();
  const stop = sessions.subscribeLifecycle(({ activeSessionId, openSessionIds }) => {
    const composer = useComposerStore.getState();
    composer.loadSession(activeSessionId);
    composer.pruneDrafts(new Set(openSessionIds));
  });
  stopSessionSync = stop;

  const initial = sessions.lifecycleSnapshot();
  useComposerStore.getState().loadSession(initial.activeSessionId);
  useComposerStore.getState().pruneDrafts(new Set(initial.openSessionIds));
  return () => {
    stop();
    if (stopSessionSync === stop) stopSessionSync = null;
  };
}

disposeOnHmr(() => {
  stopSessionSync?.();
  stopSessionSync = null;
});
