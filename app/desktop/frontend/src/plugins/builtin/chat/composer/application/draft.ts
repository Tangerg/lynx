import type { ComposerDraftInput } from "../domain/draft";

export interface ComposerDraftPort {
  replaceDraft(input: ComposerDraftInput): void;
  focusDraftEnd(textLength: number): void;
}

export function replaceDraft(input: ComposerDraftInput, port: ComposerDraftPort): void {
  port.replaceDraft(input);
  port.focusDraftEnd(input.text.length);
}
