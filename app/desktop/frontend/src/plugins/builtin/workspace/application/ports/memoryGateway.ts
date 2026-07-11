import { createSingletonPort } from "@/lib/ports/singletonPort";
export type WorkspaceMemoryScope = "cwd" | "projectRoot" | "home";

export interface WorkspaceMemoryUpdateInput {
  scope: WorkspaceMemoryScope;
  cwd?: string;
  content: string;
}

export interface WorkspaceMemoryGateway {
  save(input: WorkspaceMemoryUpdateInput): Promise<void>;
}

const port = createSingletonPort<WorkspaceMemoryGateway>(
  "Workspace memory gateway is not configured",
);

export const configureWorkspaceMemoryGateway = port.configure;
export const workspaceMemoryGateway = port.get;
