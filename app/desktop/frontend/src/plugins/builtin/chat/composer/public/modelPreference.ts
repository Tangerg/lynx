import { useComposerStore } from "../adapters/composerStore";

export interface ComposerModelPreference {
  provider: string | null;
  model: string | null;
}

export function selectedComposerModelPreference(): ComposerModelPreference {
  const { provider, model } = useComposerStore.getState();
  return { provider, model };
}

export function useComposerModelPreference(): ComposerModelPreference {
  const provider = useComposerStore((state) => state.provider);
  const model = useComposerStore((state) => state.model);
  return { provider, model };
}
