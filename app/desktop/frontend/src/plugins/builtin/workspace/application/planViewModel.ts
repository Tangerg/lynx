import type { Translate } from "@/lib/i18n";
import {
  useSessionPlan,
  type PlanStep,
  type SessionPlan,
} from "@/plugins/builtin/agent/public/plan";
import { useWorkspaceCapability } from "./workspaceCapabilities";

export type PlanState = "unavailable" | "empty" | "ready";

export interface PlanViewModel {
  steps: readonly PlanStep[];
  done: number;
  total: number;
  state: PlanState;
}

export function usePlanView(): PlanViewModel {
  // Gated by features.plan so a runtime without it shows an explicit
  // "unavailable" state rather than a perpetually-empty tab.
  return planViewModel(useWorkspaceCapability("plan"), useSessionPlan());
}

export function planViewModel(enabled: boolean, plan: SessionPlan): PlanViewModel {
  const { done, total } = plan.progress();

  return {
    steps: plan.steps,
    done,
    total,
    state: !enabled ? "unavailable" : total === 0 ? "empty" : "ready",
  };
}

export function planSubtext(
  t: Translate,
  { done, total }: Pick<PlanViewModel, "done" | "total">,
): string | undefined {
  if (total === 0) {
    return undefined;
  }
  return t("plan.complete", { done, total });
}
