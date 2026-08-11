import { queryClient } from "@/lib/queryClient";
import {
  workspaceKnowledgeGateway,
  type WorkspaceKnowledgeUpdateInput,
} from "./ports/knowledgeGateway";
import {
  WORKSPACE_KNOWLEDGE_KEY,
  useWorkspaceKnowledge as useKnowledgeQuery,
} from "./workspaceQueries";

export function useWorkspaceKnowledge(enabled: boolean, cwd?: string) {
  return useKnowledgeQuery(enabled ? { cwd } : undefined);
}

export async function saveWorkspaceKnowledge(input: WorkspaceKnowledgeUpdateInput): Promise<void> {
  await workspaceKnowledgeGateway().save(input);
  await queryClient.invalidateQueries({ queryKey: [WORKSPACE_KNOWLEDGE_KEY] });
}
