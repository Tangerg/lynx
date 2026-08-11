import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { WorkspaceKnowledgeScope } from "../workspaceQueries";

export interface WorkspaceKnowledgeUpdateInput {
  scope: WorkspaceKnowledgeScope;
  cwd?: string;
  content: string;
}

export interface WorkspaceKnowledgeGateway {
  save(input: WorkspaceKnowledgeUpdateInput): Promise<void>;
}

const port = createSingletonPort<WorkspaceKnowledgeGateway>(
  "Workspace knowledge gateway is not configured",
);

export const configureWorkspaceKnowledgeGateway = port.configure;
export const workspaceKnowledgeGateway = port.get;
