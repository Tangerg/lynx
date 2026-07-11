import { createDataQuery, createParameterizedDataQuery } from "@/lib/data/dataQuery";

export type ApprovalModeValue = "plan" | "safe" | "balanced" | "yolo";

export interface ApprovalRulesQuery {
  sessionId: string;
}

export interface ApprovalRuleInfo {
  id: string;
  scope: "session" | "project" | "global";
  tool: string;
  subject?: string;
  dir?: string;
  decision: "allow" | "deny";
}

export const APPROVAL_MODE_KEY = "approval-mode";
export const APPROVAL_RULES_KEY = "approval-rules";

export const useApprovalMode = createDataQuery<ApprovalModeValue>(APPROVAL_MODE_KEY);
export const useApprovalRules = createParameterizedDataQuery<
  ApprovalRulesQuery,
  ApprovalRuleInfo[]
>(APPROVAL_RULES_KEY);
