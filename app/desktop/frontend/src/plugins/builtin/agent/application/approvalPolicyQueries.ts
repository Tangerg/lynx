import { createDataQuery, createParameterizedDataQuery } from "@/plugins/sdk";
import type { ApprovalMode, RememberScope } from "../domain/hitl";

export interface ApprovalRulesQuery {
  sessionId: string;
}

export interface ApprovalRuleInfo {
  id: string;
  scope: RememberScope;
  tool: string;
  subject?: string;
  dir?: string;
  decision: "allow" | "deny";
}

export const APPROVAL_MODE_KEY = "approval-mode";
export const APPROVAL_RULES_KEY = "approval-rules";

export const useApprovalMode = createDataQuery<ApprovalMode>(APPROVAL_MODE_KEY);
export const useApprovalRules = createParameterizedDataQuery<
  ApprovalRulesQuery,
  ApprovalRuleInfo[]
>(APPROVAL_RULES_KEY);
