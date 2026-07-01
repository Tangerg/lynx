import { z } from "zod";
import type { ComposerImage, PastedText } from "./draft";

export interface ComposerDraft {
  value: string;
  images: ComposerImage[];
  pastes: PastedText[];
}

export type ComposerDraftArchive = Record<string, ComposerDraft>;
export type ComposerHistoryArchive = Record<string, string[]>;

export interface ComposerDraftMirror {
  activeSid: string;
  drafts: ComposerDraftArchive;
  value: string;
  images: ComposerImage[];
  pastes: PastedText[];
}

export interface ComposerHistoryState extends ComposerDraftMirror {
  history: ComposerHistoryArchive;
  histIndex: number;
  histDraft: string;
}

const persistedDraftSchema = z.object({
  drafts: z.record(z.string(), z.object({ value: z.string() })),
});

export function emptyComposerDraft(): ComposerDraft {
  return { value: "", images: [], pastes: [] };
}

export function mirrorComposerDraft(
  state: ComposerDraftMirror,
  patch: Partial<ComposerDraft>,
): Pick<ComposerDraftMirror, "value" | "images" | "pastes" | "drafts"> {
  const draft: ComposerDraft = {
    value: patch.value ?? state.value,
    images: patch.images ?? state.images,
    pastes: patch.pastes ?? state.pastes,
  };
  return { ...draft, drafts: { ...state.drafts, [state.activeSid]: draft } };
}

export function loadComposerDraft(
  state: ComposerDraftMirror,
  sessionId: string,
): Pick<
  ComposerHistoryState,
  "activeSid" | "value" | "images" | "pastes" | "histIndex" | "histDraft"
> | null {
  if (sessionId === state.activeSid) return null;
  const next = state.drafts[sessionId] ?? emptyComposerDraft();
  return {
    activeSid: sessionId,
    value: next.value,
    images: next.images,
    pastes: next.pastes,
    histIndex: -1,
    histDraft: "",
  };
}

export function pruneComposerArchives(
  state: Pick<ComposerHistoryState, "drafts" | "history" | "activeSid">,
  liveSessionIds: Set<string>,
): Pick<ComposerHistoryState, "drafts" | "history"> {
  const drafts: ComposerDraftArchive = {};
  const history: ComposerHistoryArchive = {};
  const keep = (id: string) => id === "" || id === state.activeSid || liveSessionIds.has(id);
  for (const id in state.drafts) if (keep(id)) drafts[id] = state.drafts[id]!;
  for (const id in state.history) if (keep(id)) history[id] = state.history[id]!;
  return { drafts, history };
}

export function pushComposerHistory(
  state: Pick<ComposerHistoryState, "history" | "activeSid">,
  text: string,
  cap: number,
): Pick<ComposerHistoryState, "history" | "histIndex"> | null {
  const value = text.trim();
  if (!value) return null;
  const list = state.history[state.activeSid] ?? [];
  if (list[list.length - 1] === value) return { history: state.history, histIndex: -1 };
  return {
    history: { ...state.history, [state.activeSid]: [...list, value].slice(-cap) },
    histIndex: -1,
  };
}

export function previousComposerHistory(
  state: Pick<ComposerHistoryState, "history" | "activeSid" | "histIndex" | "histDraft" | "value">,
): Pick<ComposerHistoryState, "value" | "histIndex" | "histDraft"> | null {
  const list = state.history[state.activeSid] ?? [];
  if (list.length === 0) return null;
  const histIndex = state.histIndex === -1 ? 0 : Math.min(state.histIndex + 1, list.length - 1);
  return {
    value: list[list.length - 1 - histIndex]!,
    histIndex,
    histDraft: state.histIndex === -1 ? state.value : state.histDraft,
  };
}

export function nextComposerHistory(
  state: Pick<ComposerHistoryState, "history" | "activeSid" | "histIndex" | "histDraft">,
): Pick<ComposerHistoryState, "value" | "histIndex"> | null {
  if (state.histIndex === -1) return null;
  const list = state.history[state.activeSid] ?? [];
  const histIndex = state.histIndex - 1;
  return {
    value: histIndex < 0 ? state.histDraft : list[list.length - 1 - histIndex]!,
    histIndex: histIndex < 0 ? -1 : histIndex,
  };
}

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
