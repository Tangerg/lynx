import {
  APPROVAL_MODES,
  forgetRule,
  setApprovalMode,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import {
  APPROVAL_RULES_KEY,
  type ApprovalModeValue,
  type ApprovalRuleInfo,
  useApprovalMode,
  useApprovalRules,
} from "@/lib/data/queries";
import { queryClient } from "@/lib/data/queryClient";

export type ApprovalMode = ApprovalModeValue;
export type ApprovalRuleConfig = ApprovalRuleInfo;
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

export async function forgetApprovalRules(rules: ApprovalRuleConfig[]): Promise<void> {
  for (const rule of rules) await forgetRule(rule.id);
  await queryClient.invalidateQueries({ queryKey: [APPROVAL_RULES_KEY] });
}
