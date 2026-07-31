import {
  APPROVAL_MODES,
  APPROVAL_RULES_KEY,
  forgetRule,
  setApprovalMode,
  type ApprovalMode,
  type ApprovalRuleSummary,
  useApprovalMode,
  useApprovalRules,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { queryClient } from "@/lib/queryClient";

export type { ApprovalMode, ApprovalRuleSummary };
export { APPROVAL_MODES };

export function useApprovalModeConfig() {
  return useApprovalMode();
}

export function useApprovalRuleConfigs(sessionId: string | undefined) {
  return useApprovalRules(sessionId ? { sessionId } : undefined);
}

export async function saveApprovalMode(mode: ApprovalMode): Promise<void> {
  await setApprovalMode(mode);
}

export async function forgetApprovalRule(id: string): Promise<void> {
  await forgetRule(id);
}

export async function forgetApprovalRules(rules: ApprovalRuleSummary[]): Promise<void> {
  for (const rule of rules) await forgetRule(rule.id);
  await queryClient.invalidateQueries({ queryKey: [APPROVAL_RULES_KEY] });
}
