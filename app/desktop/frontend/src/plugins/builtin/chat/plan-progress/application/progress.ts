import type { PlanStep, SessionPlan } from "@/plugins/builtin/agent/public/plan";

/**
 * The banner's own state: what the plan is, plus whether to be on screen.
 *
 * Only the visibility is decided here. Which step is current and how many are
 * done are the plan's own projection — this module used to answer both again,
 * with `currentPlanStep` disagreeing with `activePlanStep` about a plan whose
 * active step follows an untouched one.
 */
export interface PlanBannerState {
  visible: boolean;
  total: number;
  done: number;
  percent: number;
  current: PlanStep | undefined;
}

export function planBannerState(
  plan: SessionPlan,
  dismissedPlanIdentity: string | null,
): PlanBannerState {
  const { done, total } = plan.progress();
  const current = plan.activeStep();
  const dismissed = plan.identity === dismissedPlanIdentity;

  return {
    visible: current !== undefined && !dismissed,
    total,
    done,
    percent: total > 0 ? Math.round((done / total) * 100) : 0,
    current,
  };
}
