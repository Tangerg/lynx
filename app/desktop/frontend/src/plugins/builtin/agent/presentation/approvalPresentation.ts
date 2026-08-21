import type { BlockStatus } from "@/plugins/sdk/types/contentBlock";
import type { ApprovalDecision } from "../domain/hitl";

export function approvalSettledDecision(
  status: BlockStatus,
  decision: ApprovalDecision | undefined,
  pending: ApprovalDecision | null,
): ApprovalDecision | null {
  return status === "complete" ? (decision ?? null) : pending;
}

export function canSubmitApproval({
  runId,
  itemId,
  pending,
  status,
}: {
  runId?: string;
  itemId?: string;
  pending: ApprovalDecision | null;
  status: BlockStatus;
}): boolean {
  return Boolean(runId && itemId && pending === null && status === "requires-action");
}
