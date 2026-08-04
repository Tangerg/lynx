import type { PlanStep } from "@/plugins/builtin/agent/public/plan";
import { describe, expect, it } from "vitest";
import { currentPlanStep, planProgress } from "./progress";

const step = (id: number, text: string, status: PlanStep["status"]): PlanStep => ({
  id: `step-${id}`,
  text,
  status,
});

describe("planProgress", () => {
  it("prefers the in-flight step over the next pending one", () => {
    const plan = [
      step(1, "done", "done"),
      step(2, "pending", "pending"),
      step(3, "active", "active"),
    ];

    expect(currentPlanStep(plan)?.text).toBe("active");
  });

  it("falls back to the next pending step when nothing is in flight", () => {
    const plan = [step(1, "done", "done"), step(2, "next", "pending")];

    expect(currentPlanStep(plan)?.text).toBe("next");
  });

  it("summarizes completion and hides dismissed or completed plans", () => {
    const plan = [
      step(1, "done", "done"),
      step(2, "current", "active"),
      step(3, "next", "pending"),
    ];

    expect(planProgress(plan, "run-1", null)).toMatchObject({
      visible: true,
      total: 3,
      done: 1,
      percent: 33,
      current: plan[1],
    });
    expect(planProgress(plan, "run-1", "run-1").visible).toBe(false);
    expect(planProgress([step(1, "done", "done")], "run-1", null).visible).toBe(false);
  });
});
