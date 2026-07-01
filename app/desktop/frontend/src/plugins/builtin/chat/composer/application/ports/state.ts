import type { ComposerDraftInput, ComposerImage, PastedText } from "../../domain/draft";

export interface ComposerModelPreference {
  provider: string | null;
  model: string | null;
}

export interface ComposerStatePort {
  useText(): string;
  useSetText(): (value: string) => void;
  useClearDraft(): () => void;
  getText(): string;
  replaceDraft(input: ComposerDraftInput): void;
  useImages(): ComposerImage[];
  usePastes(): PastedText[];
  useAddImageFiles(): (files: File[]) => void;
  useRemoveImage(): (id: string) => void;
  useAddPaste(): (text: string) => void;
  useRemovePaste(): (id: string) => void;
  useRecordHistory(): (text: string) => void;
  recallPreviousHistory(): boolean;
  recallNextHistory(): boolean;
  getModelPreference(): ComposerModelPreference;
  useModelPreference(): ComposerModelPreference;
  useSetModelPreference(): (provider: string | null, model: string | null) => void;
}

let port: ComposerStatePort | null = null;

export function configureComposerStatePort(next: ComposerStatePort): void {
  port = next;
}

export function composerState(): ComposerStatePort {
  if (!port) throw new Error("Composer state port is not configured");
  return port;
}
