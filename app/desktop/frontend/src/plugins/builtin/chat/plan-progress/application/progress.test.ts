import { SessionPlan, type PlanStep } from "@/plugins/builtin/agent/public/plan";
import { describe, expect, it } from "vitest";
import { activePlanState } from "./progress";

const step = (id: number, text: string, status: PlanStep["status"]): PlanStep => ({
  id: `step-${id}`,
  text,
  status,
});

// Which step is current and how far the plan has got belong to the plan's own
// projection (see agent/application/view/sessionPlan). What is asserted here is
// only the banner's question: be on screen, or not.
describe("activePlanState", () => {
  const plan = [step(1, "done", "done"), step(2, "current", "active"), step(3, "next", "pending")];
  const material = SessionPlan.fromSnapshot("ses-1", 1, { revision: 3, steps: plan });

  it("reports the plan while a step is still in flight", () => {
    expect(activePlanState(material, true)).toMatchObject({
      visible: true,
      total: 3,
      done: 1,
      percent: 33,
      current: plan[1],
    });
  });

  it("stays down when the current Run is no longer active", () => {
    expect(activePlanState(material, false).visible).toBe(false);
  });

  it("stays down once the plan is finished", () => {
    const done = SessionPlan.fromSnapshot("ses-1", 1, {
      revision: 4,
      steps: [step(1, "done", "done")],
    });
    expect(activePlanState(done, true).visible).toBe(false);
  });
});
