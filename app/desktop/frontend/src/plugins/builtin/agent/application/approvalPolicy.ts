// Approval policy mutations. Reads live in this context's query module; these
// commands invalidate the matching keys after the runtime accepts the write.

import { APPROVAL_MODE_KEY, APPROVAL_RULES_KEY } from "./approvalPolicyQueries";
import type { ApprovalRuleSummary } from "./approvalPolicyQueries";
import type { ApprovalMode } from "../domain/hitl";
import { queryClient } from "@/lib/queryClient";
import { agentRuntime } from "./ports/runtimeGateway";
import { agentCommandOwner, type AgentCommandOwner } from "./agentCommandOwner";

export function setApprovalMode(mode: ApprovalMode): Promise<ApprovalMode> {
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  return owner.serializeApprovalMode(async () => {
    let saved: ApprovalMode;
    try {
      saved = await owner.settle(runtime.setApprovalMode(mode));
    } catch (error) {
      if (!owner.isCurrent()) throw error;
      await repairProjection(owner, APPROVAL_MODE_KEY);
      throw error;
    }
    owner.assertCurrent();
    queryClient.setQueryData([APPROVAL_MODE_KEY], saved);
    await repairProjection(owner, APPROVAL_MODE_KEY);
    owner.assertCurrent();
    return saved;
  });
}

/** Forget one persisted approval rule within one captured Agent generation. */
export async function forgetRule(id: string): Promise<void> {
  return forgetRules([id]);
}

export function forgetRules(ids: string[]): Promise<void> {
  const owner = agentCommandOwner();
  const runtime = agentRuntime();
  return owner.serializeApprovalRules(async () => {
    try {
      for (const id of ids) {
        await owner.settle(runtime.forgetApprovalRule(id));
        owner.assertCurrent();
        commitApprovalRuleForgotten(id);
      }
    } catch (error) {
      if (!owner.isCurrent()) throw error;
      await repairProjection(owner, APPROVAL_RULES_KEY);
      throw error;
    }
    await repairProjection(owner, APPROVAL_RULES_KEY);
    owner.assertCurrent();
  });
}

function commitApprovalRuleForgotten(id: string): void {
  queryClient.setQueriesData<ApprovalRuleSummary[]>({ queryKey: [APPROVAL_RULES_KEY] }, (current) =>
    current?.filter((rule) => rule.id !== id),
  );
}

async function repairProjection(owner: AgentCommandOwner, queryKey: string): Promise<void> {
  try {
    await owner.settle(queryClient.invalidateQueries({ queryKey: [queryKey] }));
  } catch (error) {
    if (!owner.isCurrent()) throw error;
    // An accepted response already committed its exact fact. Agent events and
    // the next read retain the projection repair path.
  }
}
