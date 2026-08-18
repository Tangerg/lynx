// The storage boundary for composer drafts: what gets written to disk, and what
// it is allowed to be when it comes back. Untrusted input is validated here
// because here is where it enters; the domain remains independent of localStorage
// and its validation library.
//
// Only the text is durable. Staged images and pastes are meant to be sent
// immediately and are heavy, so they are dropped on reload rather than persisted.

import { z } from "zod";
import type { ComposerDraftArchive } from "../domain/draftArchive";

const persistedDraftSchema = z.object({
  drafts: z.record(z.string(), z.object({ value: z.string() })),
});

export function persistedComposerDrafts(
  drafts: ComposerDraftArchive,
): Record<string, { value: string }> {
  return Object.fromEntries(
    Object.entries(drafts).map(([id, draft]) => [id, { value: draft.value }]),
  );
}

export function parsePersistedComposerDrafts(persisted: unknown): ComposerDraftArchive | null {
  const parsed = persistedDraftSchema.safeParse(persisted);
  if (!parsed.success) return null;
  const drafts: ComposerDraftArchive = {};
  for (const [id, draft] of Object.entries(parsed.data.drafts)) {
    drafts[id] = { value: draft.value, images: [], pastes: [] };
  }
  return drafts;
}
