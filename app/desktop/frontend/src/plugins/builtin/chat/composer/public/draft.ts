import { composerState } from "../application/ports/state";
import type { ComposerDraftInput } from "../domain/draft";

export type { ComposerDraftImage, ComposerDraftInput } from "../domain/draft";

export function useComposerText(): string {
  return composerState().useText();
}

export function useSetComposerText(): (value: string) => void {
  return composerState().useSetText();
}

export function useClearComposerDraft(): () => void {
  return composerState().useClearDraft();
}

export function getComposerText(): string {
  return composerState().getText();
}

export function replaceComposerDraft(input: ComposerDraftInput): void {
  composerState().replaceDraft(input);
}
