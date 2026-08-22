import { getContainer } from "@/main/container";
import { isErrorType, type KnowledgeEntry, type LyraClient } from "@/rpc";
import { KnowledgeOwner } from "../application/knowledge";
import { WorkspaceKnowledgeRevisionConflictError } from "../application/ports/knowledgeGateway";
import type { WorkspaceKnowledgeGateway } from "../application/ports/knowledgeGateway";

function runtimeKnowledgeGateway(client: LyraClient): WorkspaceKnowledgeGateway {
  return {
    async read(input) {
      const workspace = await client.workspaces.open(input.cwd ? { path: input.cwd } : undefined);
      const entry = await workspace.knowledge.get(input.scope);
      return knowledgeDocument(entry, input.scope);
    },
    async save(input) {
      const { cwd, ...update } = input;
      const workspace = await client.workspaces.open(cwd ? { path: cwd } : undefined);
      try {
        const entry = await workspace.knowledge.update(update);
        return knowledgeDocument(entry, input.scope);
      } catch (error) {
        if (isErrorType(error, "revision_conflict")) {
          throw new WorkspaceKnowledgeRevisionConflictError();
        }
        throw error;
      }
    },
  };
}

function knowledgeDocument(entry: KnowledgeEntry, scope: KnowledgeEntry["scope"]) {
  if (entry.scope !== scope) throw new Error("Workspace Knowledge response scope mismatch");
  return {
    content: entry.content,
    revision: entry.revision,
    ...(entry.updatedAt ? { updatedAt: entry.updatedAt } : {}),
  };
}

export function installWorkspaceKnowledgeGateway() {
  const gateway = runtimeKnowledgeGateway(getContainer().client());
  const owner = KnowledgeOwner.install(gateway);
  return {
    replaceRuntimeGeneration: () => owner.replaceRuntimeGeneration(),
    dispose: () => owner.dispose(),
  };
}
