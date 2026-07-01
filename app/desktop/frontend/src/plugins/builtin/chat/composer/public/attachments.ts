import type { ComposerImage, PastedText } from "../domain/draft";
import { composerState } from "../application/ports/state";

export type { ComposerImage, PastedText } from "../domain/draft";

export function useComposerImages(): ComposerImage[] {
  return composerState().useImages();
}

export function useComposerPastes(): PastedText[] {
  return composerState().usePastes();
}

export function useAddComposerImageFiles(): (files: File[]) => void {
  return composerState().useAddImageFiles();
}

export function useRemoveComposerImage(): (id: string) => void {
  return composerState().useRemoveImage();
}

export function useAddComposerPaste(): (text: string) => void {
  return composerState().useAddPaste();
}

export function useRemoveComposerPaste(): (id: string) => void {
  return composerState().useRemovePaste();
}
