import { createSingletonPort } from "@/lib/ports/singletonPort";
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

const port = createSingletonPort<ComposerStatePort>("Composer state port is not configured");

export const configureComposerStatePort = port.configure;
export const composerState = port.get;
