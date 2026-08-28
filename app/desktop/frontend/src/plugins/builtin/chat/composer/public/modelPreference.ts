import { composerState, type ComposerModelPreference } from "../application/ports/state";

export type { ComposerModelPreference } from "../application/ports/state";

export function selectedComposerModelPreference(): ComposerModelPreference {
  return composerState().getModelPreference();
}

export function useComposerModelPreference(): ComposerModelPreference {
  return composerState().useModelPreference();
}

export function useSetComposerModelPreference(): (preference: ComposerModelPreference) => void {
  return composerState().useSetModelPreference();
}
