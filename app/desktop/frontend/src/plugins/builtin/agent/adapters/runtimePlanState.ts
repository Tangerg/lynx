import type { PlanSnapshot, StateSnapshot } from "@/rpc";
import type { AgentPlanStateSnapshot, PlanStep } from "../domain/plan";

const PLAN_STATUS: Record<PlanSnapshot["status"], PlanStep["status"]> = {
  completed: "done",
  in_progress: "active",
  pending: "pending",
};

export function runtimePlanState(snapshot: StateSnapshot): AgentPlanStateSnapshot {
  return {
    type: "plan",
    revision: snapshot.revision,
    plan: snapshot.plan.map((step) => ({
      id: step.id,
      text: step.description,
      status: PLAN_STATUS[step.status],
    })),
  };
}
