import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";

export interface PlanProgress {
  visible: boolean;
  total: number;
  done: number;
  percent: number;
  current: PlanItem | null;
}

export function planProgress(
  plan: PlanItem[],
  runId: string | null,
  dismissedRunId: string | null,
): PlanProgress {
  const total = plan.length;
  const done = plan.filter((item) => item.status === "done").length;
  const current = currentPlanItem(plan);
  const hasOpenWork = plan.some((item) => item.status !== "done");
  const dismissed = runId !== null && runId === dismissedRunId;

  return {
    visible: hasOpenWork && current !== null && !dismissed,
    total,
    done,
    percent: total > 0 ? Math.round((done / total) * 100) : 0,
    current,
  };
}

export function currentPlanItem(plan: PlanItem[]): PlanItem | null {
  return (
    plan.find((item) => item.status === "doing") ??
    plan.find((item) => item.status === "todo") ??
    null
  );
}
