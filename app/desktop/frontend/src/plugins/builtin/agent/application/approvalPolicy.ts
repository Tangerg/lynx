// Approval policy mutations. Reads live in this context's query module; these
// commands invalidate the matching keys after the runtime accepts the write.

import { APPROVAL_MODE_KEY, APPROVAL_RULES_KEY } from "./approvalPolicyQueries";
import type { ApprovalMode } from "../domain/hitl";
import { queryClient } from "@/lib/queryClient";
import { agentRuntime } from "./ports/runtimeGateway";
import { agentCommandOwner } from "./agentCommandOwner";

export function setApprovalMode(mode: ApprovalMode): Promise<ApprovalMode> {
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  return owner.serializeApprovalMode(async () => {
    const saved = await runtime.setApprovalMode(mode);
    owner.assertCurrent();
    queryClient.setQueryData([APPROVAL_MODE_KEY], saved);
    await queryClient.invalidateQueries({ queryKey: [APPROVAL_MODE_KEY] });
    owner.assertCurrent();
    return saved;
  });
}

/** Forget one persisted approval rule by id (clear-all = loop the visible ids). */
export async function forgetRule(id: string): Promise<void> {
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  await runtime.forgetApprovalRule(id);
  if (!owner.isCurrent()) return;
  await queryClient.invalidateQueries({ queryKey: [APPROVAL_RULES_KEY] });
}
