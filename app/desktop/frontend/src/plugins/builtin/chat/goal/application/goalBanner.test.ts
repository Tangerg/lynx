import { describe, expect, it } from "vitest";
import { goalBudgetAxes, tightestAxis } from "./goalBanner";
import type { GoalReadModel } from "./goalQueries";

function goal(patch: Partial<GoalReadModel> = {}): GoalReadModel {
  return {
    sessionId: "ses_1",
    objective: "Ship the retry fix",
    status: "active",
    stop: null,
    budget: { maxRuns: 20, maxCostUsd: 5, maxSteps: 0 },
    used: { runs: 7, costUsd: 4.5, steps: 31 },
    ...patch,
  };
}

describe("the goal's allowance", () => {
  it("reads both sides of a capped axis and leaves an uncapped one without a fraction", () => {
    const [runs, cost, steps] = goalBudgetAxes(goal());

    expect(runs).toMatchObject({ used: 7, max: 20, spent: 0.35 });
    expect(cost).toMatchObject({ used: 4.5, max: 5, spent: 0.9, unit: "cost" });
    // The wire omits a cap it does not have, and an uncapped axis gets no bar —
    // a full track under "no limit" would read as "nearly spent".
    expect(steps).toMatchObject({ used: 31, max: 0, spent: undefined });
  });

  it("names the axis that will stop the loop first, not the biggest number", () => {
    // Steps is the largest count and runs the largest cap; cost is the one at 90%.
    expect(tightestAxis(goalBudgetAxes(goal()))?.label).toBe("goal.budget.cost");
  });

  it("has no tightest axis when nothing is capped", () => {
    const uncapped = goal({ budget: { maxRuns: 0, maxCostUsd: 0, maxSteps: 0 } });
    expect(tightestAxis(goalBudgetAxes(uncapped))).toBeUndefined();
  });
});
