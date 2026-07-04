import type { PlanItem } from "@/plugins/builtin/agent/public/viewState";
import { describe, expect, it } from "vitest";
import { currentPlanItem, planProgress } from "./progress";

const item = (id: number, text: string, status: PlanItem["status"]): PlanItem => ({
  id,
  pid: `step-${id}`,
  text,
  status,
});

describe("planProgress", () => {
  it("prefers the in-flight item over the next todo", () => {
    const plan = [item(1, "done", "done"), item(2, "todo", "todo"), item(3, "doing", "doing")];

    expect(currentPlanItem(plan)?.text).toBe("doing");
  });

  it("falls back to the next todo when nothing is in flight", () => {
    const plan = [item(1, "done", "done"), item(2, "next", "todo")];

    expect(currentPlanItem(plan)?.text).toBe("next");
  });

  it("summarizes completion and hides dismissed or completed plans", () => {
    const plan = [item(1, "done", "done"), item(2, "current", "doing"), item(3, "next", "todo")];

    expect(planProgress(plan, "run-1", null)).toMatchObject({
      visible: true,
      total: 3,
      done: 1,
      percent: 33,
      current: plan[1],
    });
    expect(planProgress(plan, "run-1", "run-1").visible).toBe(false);
    expect(planProgress([item(1, "done", "done")], "run-1", null).visible).toBe(false);
  });
});
