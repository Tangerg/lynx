// Approval-stance mutations (B9, docs/613) — the global approval mode + the
// per-session remembered-decision management. Thin wrappers over the client
// that invalidate the matching react-query keys so the Approvals pane re-reads.
// (Mirrors lib/agent/useProviderConfig: mutation lives here, reads in lib/data.)

import { APPROVAL_MODE_KEY, type ApprovalModeValue, REMEMBERED_KEY } from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";
import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";

export async function setApprovalMode(mode: ApprovalModeValue): Promise<void> {
  await getContainer().client().approval.setMode(mode);
  await queryClient.invalidateQueries({ queryKey: [APPROVAL_MODE_KEY] });
}

/** Clear one remembered tool decision, or — `tool` omitted — all of them for the session. */
export async function forgetDecision(sessionId: string, tool?: string): Promise<void> {
  await getContainer()
    .client()
    .approval.forget({ sessionId: asSessionId(sessionId), tool });
  await queryClient.invalidateQueries({ queryKey: [REMEMBERED_KEY] });
}
