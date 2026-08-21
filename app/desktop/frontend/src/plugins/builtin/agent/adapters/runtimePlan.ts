import type { Plan as RuntimePlan, PlanStep as RuntimePlanStep } from "@/rpc";
import type { AgentPlan, PlanStep } from "@/plugins/sdk/types/agentSessionView";

const PLAN_STATUS: Record<RuntimePlanStep["status"], PlanStep["status"]> = {
  completed: "done",
  in_progress: "active",
  pending: "pending",
};

export function runtimePlan(plan: RuntimePlan): AgentPlan {
  return {
    revision: plan.revision,
    steps: plan.steps.map((step) => ({
      id: step.id,
      text: step.description,
      status: PLAN_STATUS[step.status],
    })),
  };
}
