// Approval-stance mutations (B9, 613) — the global approval mode + the
// per-session remembered-decision management. Thin wrappers over the client
// that invalidate the matching react-query keys so the Approvals pane re-reads.
// (Mirrors lib/agent/useProviderConfig: mutation lives here, reads in lib/data.)

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
