import { getContainer } from "@/main/container";
import { isErrorType } from "@/rpc";
import {
  configureWorkspaceKnowledgeGateway,
  WorkspaceKnowledgeRevisionConflictError,
} from "../application/ports/knowledgeGateway";
import type { WorkspaceKnowledgeGateway } from "../application/ports/knowledgeGateway";

const gateway: WorkspaceKnowledgeGateway = {
  async read(input) {
    const workspace = await getContainer()
      .client()
      .workspaces.open(input.cwd ? { path: input.cwd } : undefined);
    const entry = await workspace.knowledge.get(input.scope);
    return {
      content: entry.content,
      revision: entry.revision,
      ...(entry.updatedAt ? { updatedAt: entry.updatedAt } : {}),
    };
  },
  async save(input) {
    const { cwd, ...update } = input;
    const workspace = await getContainer()
      .client()
      .workspaces.open(cwd ? { path: cwd } : undefined);
    try {
      const entry = await workspace.knowledge.update(update);
      return {
        content: entry.content,
        revision: entry.revision,
        ...(entry.updatedAt ? { updatedAt: entry.updatedAt } : {}),
      };
    } catch (error) {
      if (isErrorType(error, "revision_conflict")) {
        throw new WorkspaceKnowledgeRevisionConflictError();
      }
      throw error;
    }
  },
};

export function installWorkspaceKnowledgeGateway(): () => void {
  return configureWorkspaceKnowledgeGateway(gateway);
}
