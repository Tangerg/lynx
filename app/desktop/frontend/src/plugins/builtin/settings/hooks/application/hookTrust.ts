// Lifecycle-hook trust mutation. A cloned repo's project hooks stay inert until
// the user trusts the project here; the toggle takes effect on the next turn.

import { HOOKS_KEY } from "./hookQueries";
import { queryClient } from "@/lib/data/queryClient";
import { hookTrustGateway } from "./ports/hookTrustGateway";

export async function setHookTrust(projectRoot: string, trusted: boolean): Promise<void> {
  await hookTrustGateway().setProjectTrust(projectRoot, trusted);
  await queryClient.invalidateQueries({ queryKey: [HOOKS_KEY] });
}
