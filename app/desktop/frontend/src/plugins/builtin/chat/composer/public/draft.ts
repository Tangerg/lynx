import { replaceDraft } from "../application/draft";
import { composerDraftPort } from "../adapters/composerDraftPort";
import type { ComposerDraftInput } from "../domain/draft";

export type { ComposerDraftImage, ComposerDraftInput } from "../domain/draft";

export function replaceComposerDraft(input: ComposerDraftInput): void {
  replaceDraft(input, composerDraftPort);
}
