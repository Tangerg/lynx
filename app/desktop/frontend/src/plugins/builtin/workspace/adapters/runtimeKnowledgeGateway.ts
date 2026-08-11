import { getContainer } from "@/main/container";
import { configureWorkspaceKnowledgeGateway } from "../application/ports/knowledgeGateway";
import type { WorkspaceKnowledgeGateway } from "../application/ports/knowledgeGateway";

const gateway: WorkspaceKnowledgeGateway = {
  async read(input) {
    const workspace = await getContainer()
      .client()
      .workspaces.open(input.cwd ? { path: input.cwd } : undefined);
    const entry = await workspace.knowledge.get(input.scope);
    return {
      content: entry.content,
      ...(entry.updatedAt ? { updatedAt: entry.updatedAt } : {}),
    };
  },
  async save(input) {
    const { cwd, ...update } = input;
    const workspace = await getContainer()
      .client()
      .workspaces.open(cwd ? { path: cwd } : undefined);
    await workspace.knowledge.update(update);
  },
};

export function installWorkspaceKnowledgeGateway(): () => void {
  return configureWorkspaceKnowledgeGateway(gateway);
}
