import { createSingletonPort } from "@/lib/ports/singletonPort";
export interface WorkspaceErrorClassifier {
  isVcsUnavailable(error: unknown): boolean;
}

const port = createSingletonPort<WorkspaceErrorClassifier>(
  "Workspace error classifier is not configured",
);

export const configureWorkspaceErrorClassifier = port.configure;
export const workspaceErrorClassifier = port.get;
