// Lifecycle-hook trust mutation (workspace.hooks.setTrust). A cloned repo's
// project hooks stay inert until the user trusts the project here; the toggle
// takes effect on the next turn (the runtime re-reads trust per turn). Thin
// wrapper over the client that invalidates the hooks view so the `active` flags
// flip immediately.

import { HOOKS_KEY } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { getContainer } from "@/main/container";

export async function setHookTrust(projectRoot: string, trusted: boolean): Promise<void> {
  await getContainer().client().workspace.hooks.setTrust(projectRoot, trusted);
  await queryClient.invalidateQueries({ queryKey: [HOOKS_KEY] });
}
