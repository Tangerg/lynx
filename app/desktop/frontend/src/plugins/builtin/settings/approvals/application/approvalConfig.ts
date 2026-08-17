import {
  APPROVAL_MODES,
  agentCommandWasRetired,
  forgetRule,
  forgetRules,
  setApprovalMode,
  type ApprovalMode,
  type ApprovalRuleSummary,
  useApprovalMode,
  useApprovalRules,
} from "@/plugins/builtin/agent/public/approvalPolicy";

export type { ApprovalMode, ApprovalRuleSummary };
export { APPROVAL_MODES, agentCommandWasRetired };

export function useApprovalModeConfig() {
  return useApprovalMode();
}

export function useApprovalRuleConfigs(sessionId: string | undefined) {
  return useApprovalRules(sessionId ? { sessionId } : undefined);
}

export function saveApprovalMode(mode: ApprovalMode): Promise<ApprovalMode> {
  return setApprovalMode(mode);
}

export async function forgetApprovalRule(id: string): Promise<void> {
  await forgetRule(id);
}

export async function forgetApprovalRules(rules: ApprovalRuleSummary[]): Promise<void> {
  return forgetRules(rules.map((rule) => rule.id));
}
