// Shared VCS three-state gate (AUX_API §2.1) for the Files / Diff views:
// no git binary → features.git=false (never call); git but non-repo cwd →
// vcs_unavailable (an EXPECTED state with its own copy, not the error
// branch); clean repo → the view's own empty state.

import type { IconName } from "@/components/common";
import { errorType, RpcError } from "@/rpc";

export function isVcsUnavailable(error: unknown): boolean {
  return error instanceof RpcError && errorType(error.data) === "vcs_unavailable";
}

export const gitOffEmpty = (icon: IconName) => ({
  icon,
  title: "Git not available",
  sub: "This runtime has no git binary on its PATH.",
});

export const notARepoEmpty = (icon: IconName) => ({
  icon,
  title: "Not a git repository",
  sub: "The session's working directory is not under version control.",
});
