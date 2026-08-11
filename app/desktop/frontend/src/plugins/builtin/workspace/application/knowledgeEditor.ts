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
  savedContent: string,
): KnowledgeEditorState {
  return { content: savedContent, draft: state.draft };
}

export function isKnowledgeDirty(state: KnowledgeEditorState): boolean {
  return state.draft !== state.content;
}
