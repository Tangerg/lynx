import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import { SessionPlan, type PlanStep } from "@/plugins/builtin/agent/public/plan";
import { planSubtext, planViewModel } from "./planViewModel";

const step = (over: Partial<PlanStep>): PlanStep => ({
  id: "p1",
  status: "pending",
  text: "Inspect workspace",
  ...over,
});

const plan = (steps: readonly PlanStep[]) =>
  SessionPlan.fromSnapshot("ses-plan", 1, { revision: 1, steps });

describe("planViewModel", () => {
  it("counts completed steps without reordering the plan", () => {
    const first = step({ id: "p1", status: "done" });
    const second = step({ id: "p2", status: "active" });
    const third = step({ id: "p3", status: "pending" });

    expect(planViewModel(true, plan([first, second, third]))).toEqual({
      steps: [first, second, third],
      done: 1,
      total: 3,
      state: "ready",
    });
  });

  it("projects an empty plan", () => {
    expect(planViewModel(true, plan([]))).toEqual({
      steps: [],
      done: 0,
      total: 0,
      state: "empty",
    });
  });

  // A runtime that never negotiated features.plan has no plan to be empty OF, and
  // saying "no plan yet" there reads as "the agent hasn't planned", not "this build
  // cannot".
  it("reports an ungated runtime as unavailable rather than empty", () => {
    expect(planViewModel(false, plan([])).state).toBe("unavailable");
  });
});

describe("planSubtext", () => {
  it("omits header subtext for an empty plan", () => {
    expect(planSubtext(t, { done: 0, total: 0 })).toBeUndefined();
  });

  it("builds completion subtext", () => {
    expect(planSubtext(t, { done: 2, total: 3 })).toBe("2 of 3 complete");
  });
});
