import type { PlanStep } from "@/plugins/builtin/agent/public/plan";

export interface PlanProgress {
  visible: boolean;
  total: number;
  done: number;
  percent: number;
  current: PlanStep | null;
}

export function planProgress(
  steps: readonly PlanStep[],
  runId: string | null,
  dismissedRunId: string | null,
): PlanProgress {
  const total = steps.length;
  const done = steps.filter((step) => step.status === "done").length;
  const current = currentPlanStep(steps);
  const dismissed = runId !== null && runId === dismissedRunId;

  return {
    visible: done < total && current !== null && !dismissed,
    total,
    done,
    percent: total > 0 ? Math.round((done / total) * 100) : 0,
    current,
  };
}

export function currentPlanStep(steps: readonly PlanStep[]): PlanStep | null {
  return (
    steps.find((step) => step.status === "active") ??
    steps.find((step) => step.status === "pending") ??
    null
  );
}
