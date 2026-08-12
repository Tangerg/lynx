import type { WorkspaceKnowledgeDocument } from "./ports/knowledgeGateway";

export interface KnowledgeEditorState extends WorkspaceKnowledgeDocument {
  draft: string;
}

export function openedKnowledgeEditor(snapshot: WorkspaceKnowledgeDocument): KnowledgeEditorState {
  return { ...snapshot, draft: snapshot.content };
}

export function editKnowledge(state: KnowledgeEditorState, draft: string): KnowledgeEditorState {
  return { ...state, draft };
}

/** Advance the saved baseline without erasing edits made while the write was in flight. */
export function commitKnowledgeSave(
  state: KnowledgeEditorState,
  saved: WorkspaceKnowledgeDocument,
): KnowledgeEditorState {
  return { ...saved, draft: state.draft };
}

/** Fold a refetched list row without destroying an unsaved local draft. */
export function reconcileKnowledgeSnapshot(
  state: KnowledgeEditorState,
  snapshot: WorkspaceKnowledgeDocument,
): KnowledgeEditorState {
  if (state.revision === snapshot.revision) return state;
  return isKnowledgeDirty(state) ? state : openedKnowledgeEditor(snapshot);
}

/** Settle a save against the newest list snapshot observed while it was in flight. */
export function settleKnowledgeSave(
  state: KnowledgeEditorState,
  saved: WorkspaceKnowledgeDocument,
  latest: WorkspaceKnowledgeDocument,
): KnowledgeEditorState {
  const committed = commitKnowledgeSave(state, saved);
  // The list may still be the pre-save snapshot. Only a third revision proves
  // that a later observation raced the response; opaque revisions have no
  // ordering, so equality is the only comparison we make.
  if (latest.revision === state.revision || latest.revision === saved.revision) return committed;
  return reconcileKnowledgeSnapshot(committed, latest);
}

/** Adopt the latest revision after a CAS conflict while preserving user intent. */
export function rebaseKnowledgeDraft(
  state: KnowledgeEditorState,
  snapshot: WorkspaceKnowledgeDocument,
): KnowledgeEditorState {
  return { ...snapshot, draft: state.draft };
}

export function isKnowledgeDirty(state: KnowledgeEditorState): boolean {
  return state.draft !== state.content;
}
