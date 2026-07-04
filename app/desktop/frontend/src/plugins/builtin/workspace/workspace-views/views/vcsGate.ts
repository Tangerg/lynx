// Shared VCS three-state gate (AUX_API §2.1) for the Files / Diff views:
// no git binary → features.git=false (never call); git but non-repo cwd →
// vcs_unavailable (an EXPECTED state with its own copy, not the error
// branch); clean repo → the view's own empty state.

import type { IconName } from "@/ui";
import { t } from "@/lib/i18n";
export { isVcsUnavailable } from "@/plugins/builtin/workspace/application/vcsAvailability";

export const gitOffEmpty = (icon: IconName) => ({
  icon,
  title: t("vcs.gitNotAvailable"),
  sub: t("vcs.gitNotAvailableSub"),
});

export const notARepoEmpty = (icon: IconName) => ({
  icon,
  title: t("vcs.notARepo"),
  sub: t("vcs.notARepoSub"),
});
