// Approval policy mutations. Reads live in this context's query module; these
// commands invalidate the matching keys after the runtime accepts the write.

import { APPROVAL_MODE_KEY, APPROVAL_RULES_KEY } from "./approvalPolicyQueries";
import type { ApprovalMode } from "../domain/hitl";
import { queryClient } from "@/lib/queryClient";
import { createSerialTaskQueue } from "@/lib/serialTaskQueue";
import { agentRuntime } from "./ports/runtimeGateway";

const modeChanges = createSerialTaskQueue();

export function setApprovalMode(mode: ApprovalMode): Promise<ApprovalMode> {
  return modeChanges.run(async () => {
    const saved = await agentRuntime().setApprovalMode(mode);
    queryClient.setQueryData([APPROVAL_MODE_KEY], saved);
    await queryClient.invalidateQueries({ queryKey: [APPROVAL_MODE_KEY] });
    return saved;
  });
}

/** Forget one persisted approval rule by id (clear-all = loop the visible ids). */
export async function forgetRule(id: string): Promise<void> {
  await agentRuntime().forgetApprovalRule(id);
  await queryClient.invalidateQueries({ queryKey: [APPROVAL_RULES_KEY] });
}
