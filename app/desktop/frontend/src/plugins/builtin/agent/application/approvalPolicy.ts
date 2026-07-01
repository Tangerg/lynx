// Approval policy mutations. Reads live in lib/data query hooks; these commands
// invalidate the matching keys after the runtime accepts the write.

import { APPROVAL_MODE_KEY, APPROVAL_RULES_KEY, type ApprovalModeValue } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { getContainer } from "@/main/container";

export async function setApprovalMode(mode: ApprovalModeValue): Promise<void> {
  await getContainer().client().approval.setMode(mode);
  await queryClient.invalidateQueries({ queryKey: [APPROVAL_MODE_KEY] });
}

/** Forget one persisted approval rule by id (clear-all = loop the visible ids). */
export async function forgetRule(id: string): Promise<void> {
  await getContainer().client().approval.forgetRule(id);
  await queryClient.invalidateQueries({ queryKey: [APPROVAL_RULES_KEY] });
}
