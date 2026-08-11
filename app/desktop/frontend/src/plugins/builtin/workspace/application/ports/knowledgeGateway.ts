import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { WorkspaceKnowledgeScope } from "../workspaceQueries";

export interface WorkspaceKnowledgeUpdateInput {
  scope: WorkspaceKnowledgeScope;
  cwd?: string;
  content: string;
}

export interface WorkspaceKnowledgeReadInput {
  scope: WorkspaceKnowledgeScope;
  cwd?: string;
}

export interface WorkspaceKnowledgeDocument {
  content: string;
  updatedAt?: string;
}

export interface WorkspaceKnowledgeGateway {
  read(input: WorkspaceKnowledgeReadInput): Promise<WorkspaceKnowledgeDocument>;
  save(input: WorkspaceKnowledgeUpdateInput): Promise<void>;
}

const port = createSingletonPort<WorkspaceKnowledgeGateway>(
  "Workspace knowledge gateway is not configured",
);

export const configureWorkspaceKnowledgeGateway = port.configure;
export const workspaceKnowledgeGateway = port.get;
