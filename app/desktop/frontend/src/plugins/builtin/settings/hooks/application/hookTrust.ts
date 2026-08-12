// Lifecycle-hook trust mutation. A cloned repo's project hooks stay inert until
// the user trusts the project here; the toggle takes effect on the next turn.

import { HOOKS_KEY } from "./hookQueries";
import { queryClient } from "@/lib/queryClient";
import { createKeyedSerialTaskQueue } from "@/lib/serialTaskQueue";
import { hookTrustGateway } from "./ports/hookTrustGateway";

const hookTrustMutations = createKeyedSerialTaskQueue<string>();

export async function setHookTrust(projectRoot: string, trusted: boolean): Promise<void> {
  return hookTrustMutations.run(projectRoot, async () => {
    try {
      await hookTrustGateway().setProjectTrust(projectRoot, trusted);
    } catch (error) {
      await queryClient.invalidateQueries({ queryKey: [HOOKS_KEY] }).catch(() => undefined);
      throw error;
    }
    // Trust is committed before the read settles. A failed revalidation must
    // not misreport the security mutation as failed; hooks.changed and the
    // next query retry remain authoritative repair paths.
    await queryClient.invalidateQueries({ queryKey: [HOOKS_KEY] }).catch(() => undefined);
  });
}
