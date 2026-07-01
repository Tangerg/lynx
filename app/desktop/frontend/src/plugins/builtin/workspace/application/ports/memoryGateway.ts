export type WorkspaceMemoryScope = "cwd" | "projectRoot" | "home";

export interface WorkspaceMemoryUpdateInput {
  scope: WorkspaceMemoryScope;
  cwd?: string;
  content: string;
}

export interface WorkspaceMemoryGateway {
  save(input: WorkspaceMemoryUpdateInput): Promise<void>;
}

let port: WorkspaceMemoryGateway | null = null;

export function configureWorkspaceMemoryGateway(next: WorkspaceMemoryGateway): void {
  port = next;
}

export function workspaceMemoryGateway(): WorkspaceMemoryGateway {
  if (!port) throw new Error("Workspace memory gateway is not configured");
  return port;
}
