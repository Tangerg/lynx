import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { WorkspaceKnowledgeScope } from "../workspaceQueries";

export interface WorkspaceKnowledgeUpdateInput {
  scope: WorkspaceKnowledgeScope;
  cwd?: string;
  content: string;
  expectedRevision: string;
}

export interface WorkspaceKnowledgeReadInput {
  scope: WorkspaceKnowledgeScope;
  cwd?: string;
}

export interface WorkspaceKnowledgeDocument {
  content: string;
  revision: string;
  updatedAt?: string;
}

export interface WorkspaceKnowledgeGateway {
  read(input: WorkspaceKnowledgeReadInput): Promise<WorkspaceKnowledgeDocument>;
  save(input: WorkspaceKnowledgeUpdateInput): Promise<WorkspaceKnowledgeDocument>;
}

/** Neutral application failure; the Runtime adapter owns the wire problem type. */
export class WorkspaceKnowledgeRevisionConflictError extends Error {
  constructor() {
    super();
    this.name = "WorkspaceKnowledgeRevisionConflictError";
  }
}

const port = createSingletonPort<WorkspaceKnowledgeGateway>(
  "Workspace knowledge gateway is not configured",
);

export const configureWorkspaceKnowledgeGateway = port.configure;
export const workspaceKnowledgeGateway = port.get;
