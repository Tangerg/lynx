import { queryClient } from "@/lib/queryClient";
import {
  workspaceKnowledgeGateway,
  WorkspaceKnowledgeRevisionConflictError,
  type WorkspaceKnowledgeDocument,
  type WorkspaceKnowledgeReadInput,
  type WorkspaceKnowledgeUpdateInput,
} from "./ports/knowledgeGateway";
import {
  WORKSPACE_KNOWLEDGE_KEY,
  useWorkspaceKnowledge as useKnowledgeQuery,
  type WorkspaceKnowledgeQuery,
} from "./workspaceQueries";

export function useWorkspaceKnowledge(query: WorkspaceKnowledgeQuery | undefined) {
  return useKnowledgeQuery(query);
}

export function loadWorkspaceKnowledge(
  input: WorkspaceKnowledgeReadInput,
): Promise<WorkspaceKnowledgeDocument> {
  return workspaceKnowledgeGateway().read(input);
}

export async function saveWorkspaceKnowledge(
  input: WorkspaceKnowledgeUpdateInput,
): Promise<WorkspaceKnowledgeDocument> {
  try {
    const saved = await workspaceKnowledgeGateway().save(input);
    // The mutation response is authoritative. Cache revalidation is settlement
    // repair for other views/windows and must not turn a committed write into an
    // apparent command failure.
    await queryClient
      .invalidateQueries({ queryKey: [WORKSPACE_KNOWLEDGE_KEY] })
      .catch(() => undefined);
    return saved;
  } catch (error) {
    // A lost response is ambiguous, while a revision conflict means the list is
    // definitely stale. Revalidate in both cases but preserve the command error.
    await queryClient
      .invalidateQueries({ queryKey: [WORKSPACE_KNOWLEDGE_KEY] })
      .catch(() => undefined);
    throw error;
  }
}

export function isWorkspaceKnowledgeRevisionConflict(
  error: unknown,
): error is WorkspaceKnowledgeRevisionConflictError {
  return error instanceof WorkspaceKnowledgeRevisionConflictError;
}
