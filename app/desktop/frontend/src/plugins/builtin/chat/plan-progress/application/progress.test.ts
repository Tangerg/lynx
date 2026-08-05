import type { PlanStep } from "@/plugins/builtin/agent/public/plan";
import { describe, expect, it } from "vitest";
import { planBannerState } from "./progress";

const step = (id: number, text: string, status: PlanStep["status"]): PlanStep => ({
  id: `step-${id}`,
  text,
  status,
});

// Which step is current and how far the plan has got belong to the plan's own
// projection (see agent/application/view/sessionPlan). What is asserted here is
// only the banner's question: be on screen, or not.
describe("planBannerState", () => {
  const plan = [step(1, "done", "done"), step(2, "current", "active"), step(3, "next", "pending")];

  it("reports the plan while a step is still in flight", () => {
    expect(planBannerState(plan, "run-1", null)).toMatchObject({
      visible: true,
      total: 3,
      done: 1,
      percent: 33,
      current: plan[1],
    });
  });

  it("stays down for the run the reader dismissed it on", () => {
    expect(planBannerState(plan, "run-1", "run-1").visible).toBe(false);
  });

  it("stays down once the plan is finished", () => {
    expect(planBannerState([step(1, "done", "done")], "run-1", null).visible).toBe(false);
  });
});
