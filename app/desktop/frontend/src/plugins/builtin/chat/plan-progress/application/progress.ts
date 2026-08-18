import type { PlanStep, SessionPlan } from "@/plugins/builtin/agent/public/plan";

/**
 * The banner's own state: what the plan is, plus whether to be on screen.
 *
 * Only the visibility is decided here. Which step is current and how many are
 * done are the Plan projection's facts and are not re-derived here.
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
