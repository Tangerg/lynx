import { SessionPlan, type PlanStep } from "@/plugins/builtin/agent/public/plan";
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
  const material = SessionPlan.fromSnapshot("ses-1", 1, { revision: 3, plan });

  it("reports the plan while a step is still in flight", () => {
    expect(planBannerState(material, null)).toMatchObject({
      visible: true,
      total: 3,
      done: 1,
      percent: 33,
      current: plan[1],
    });
  });

  it("stays down for the exact Plan replacement the reader dismissed", () => {
    expect(planBannerState(material, material.identity).visible).toBe(false);
  });

  it("stays down once the plan is finished", () => {
    const done = SessionPlan.fromSnapshot("ses-1", 1, {
      revision: 4,
      plan: [step(1, "done", "done")],
    });
    expect(planBannerState(done, null).visible).toBe(false);
  });
});
