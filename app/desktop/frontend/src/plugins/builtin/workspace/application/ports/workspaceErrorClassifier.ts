export interface WorkspaceErrorClassifier {
  isVcsUnavailable(error: unknown): boolean;
}

let port: WorkspaceErrorClassifier | null = null;

export function configureWorkspaceErrorClassifier(next: WorkspaceErrorClassifier): void {
  port = next;
}

export function workspaceErrorClassifier(): WorkspaceErrorClassifier {
  if (!port) throw new Error("Workspace error classifier is not configured");
  return port;
}
