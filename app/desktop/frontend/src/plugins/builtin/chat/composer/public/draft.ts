import { replaceDraft } from "../application/draft";
import { composerDraftPort } from "../adapters/composerDraftPort";
import { useComposerStore } from "../adapters/composerStore";
import type { ComposerDraftInput } from "../domain/draft";

export type { ComposerDraftImage, ComposerDraftInput } from "../domain/draft";

export function useComposerText(): string {
  return useComposerStore((state) => state.value);
}

export function useSetComposerText(): (value: string) => void {
  return useComposerStore((state) => state.setValue);
}

export function useClearComposerDraft(): () => void {
  return useComposerStore((state) => state.clear);
}

export function getComposerText(): string {
  return useComposerStore.getState().value;
}

export function replaceComposerDraft(input: ComposerDraftInput): void {
  replaceDraft(input, composerDraftPort);
}
