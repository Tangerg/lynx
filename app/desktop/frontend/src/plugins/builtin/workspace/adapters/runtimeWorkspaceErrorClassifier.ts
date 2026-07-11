import { isErrorType } from "@/rpc";
import { configureWorkspaceErrorClassifier } from "../application/ports/workspaceErrorClassifier";
import type { WorkspaceErrorClassifier } from "../application/ports/workspaceErrorClassifier";

const classifier: WorkspaceErrorClassifier = {
  isVcsUnavailable(error) {
    return isErrorType(error, "vcs_unavailable");
  },
};

export function installWorkspaceErrorClassifier(): () => void {
  return configureWorkspaceErrorClassifier(classifier);
}
