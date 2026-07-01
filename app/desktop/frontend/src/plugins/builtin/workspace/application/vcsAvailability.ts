import { workspaceErrorClassifier } from "./ports/workspaceErrorClassifier";

export function isVcsUnavailable(error: unknown): boolean {
  return workspaceErrorClassifier().isVcsUnavailable(error);
}
