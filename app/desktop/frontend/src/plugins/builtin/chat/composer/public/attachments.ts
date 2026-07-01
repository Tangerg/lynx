import { useComposerStore } from "../adapters/composerStore";
import type { ComposerImage, PastedText } from "../domain/draft";

export type { ComposerImage, PastedText } from "../domain/draft";

export function useComposerImages(): ComposerImage[] {
  return useComposerStore((state) => state.images);
}

export function useComposerPastes(): PastedText[] {
  return useComposerStore((state) => state.pastes);
}

export function useAddComposerImageFiles(): (files: File[]) => void {
  return useComposerStore((state) => state.addImageFiles);
}

export function useRemoveComposerImage(): (id: string) => void {
  return useComposerStore((state) => state.removeImage);
}

export function useAddComposerPaste(): (text: string) => void {
  return useComposerStore((state) => state.addPaste);
}

export function useRemoveComposerPaste(): (id: string) => void {
  return useComposerStore((state) => state.removePaste);
}
