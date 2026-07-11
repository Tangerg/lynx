// Approval policy mutations. Reads live in this context's query module; these
// commands invalidate the matching keys after the runtime accepts the write.

import {
  APPROVAL_MODE_KEY,
  APPROVAL_RULES_KEY,
  type ApprovalModeValue,
} from "./approvalPolicyQueries";
import { queryClient } from "@/lib/data/queryClient";
import { agentRuntime } from "./ports/runtimeGateway";

export async function setApprovalMode(mode: ApprovalModeValue): Promise<void> {
  await agentRuntime().setApprovalMode(mode);
  await queryClient.invalidateQueries({ queryKey: [APPROVAL_MODE_KEY] });
}

/** Forget one persisted approval rule by id (clear-all = loop the visible ids). */
export async function forgetRule(id: string): Promise<void> {
  await agentRuntime().forgetApprovalRule(id);
  await queryClient.invalidateQueries({ queryKey: [APPROVAL_RULES_KEY] });
}
