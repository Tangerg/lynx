import type { BlockStatus } from "@/plugins/builtin/agent/public/viewState";
import type { ApprovalDecision } from "../domain/hitl";
export { dangerHints } from "./dangerPatterns";

export type ApprovalRisk = "low" | "medium" | "high";
export type ApprovalTone = "neutral" | "warning" | "danger";

export interface ApprovalRiskView {
  risk: ApprovalRisk;
  labelKey: string;
  tone: ApprovalTone;
}

export interface ApprovalScopeView {
  scope: string;
  tone: ApprovalTone;
}

export interface ApprovalReversibilityView {
  labelKey: string;
  tone: ApprovalTone;
}

const RISK_VIEW: Record<ApprovalRisk, ApprovalRiskView> = {
  low: { risk: "low", labelKey: "approval.risk.low", tone: "neutral" },
  medium: { risk: "medium", labelKey: "approval.risk.medium", tone: "warning" },
  high: { risk: "high", labelKey: "approval.risk.high", tone: "danger" },
};

const SCOPE_TONE: Record<string, ApprovalTone> = {
  read: "neutral",
  write: "warning",
  network: "neutral",
  shell: "warning",
  delete: "danger",
};

export function approvalRiskView(risk?: ApprovalRisk): ApprovalRiskView {
  return RISK_VIEW[risk ?? "medium"];
}

export function approvalScopeViews(scopes: readonly string[] | undefined): ApprovalScopeView[] {
  return (scopes ?? []).map((scope) => ({ scope, tone: SCOPE_TONE[scope] ?? "neutral" }));
}

export function approvalReversibilityView(
  reversible: boolean | undefined,
): ApprovalReversibilityView | null {
  if (reversible === undefined) return null;
  return reversible
    ? { labelKey: "approval.reversible", tone: "neutral" }
    : { labelKey: "approval.permanent", tone: "danger" };
}

export function approvalSettledDecision(
  status: BlockStatus,
  decision: ApprovalDecision | undefined,
  pending: ApprovalDecision | null,
): ApprovalDecision | null {
  return status === "complete" ? (decision ?? null) : pending;
}

export function canSubmitApproval({
  parentRunId,
  itemId,
  pending,
  status,
}: {
  parentRunId?: string;
  itemId?: string;
  pending: ApprovalDecision | null;
  status: BlockStatus;
}): boolean {
  return Boolean(parentRunId && itemId && pending === null && status === "requires-action");
}
