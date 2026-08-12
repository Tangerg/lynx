import { describe, expect, it } from "vitest";
import { en } from "@/lib/i18n/locales/en";
import {
  GOAL_STATUS_I18N,
  GOAL_STOP_I18N,
  goalBudgetAxes,
  goalCanResume,
  tightestAxis,
} from "./goalBanner";
import type { GoalReadModel } from "./goalQueries";

function goal(patch: Partial<GoalReadModel> = {}): GoalReadModel {
  return {
    sessionId: "ses_1",
    objective: "Ship the retry fix",
    status: "active",
    stop: null,
    budget: { maxRuns: 20, maxCostUsd: 5, maxSteps: 0 },
    used: { runs: 7, costUsd: 4.5, steps: 31 },
    provider: "openai",
    model: "gpt-5",
    createdAt: "2026-08-12T08:00:00Z",
    updatedAt: "2026-08-12T08:01:00Z",
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

describe("goal lifecycle actions", () => {
  it.each(["runBudgetReached", "costBudgetReached", "stepBudgetReached"] as const)(
    "does not offer a guaranteed-failing resume after %s",
    (code) => {
      expect(
        goalCanResume(
          goal({
            status: "blocked",
            stop: { code, detail: "" },
          }),
        ),
      ).toBe(false);
    },
  );

  it.each(["stoppedByUser", "awaitingInput", "blockedByModel"] as const)(
    "keeps %s resumable",
    (code) => {
      expect(
        goalCanResume(
          goal({
            status: code === "blockedByModel" ? "blocked" : "paused",
            stop: { code, detail: "" },
          }),
        ),
      ).toBe(true);
    },
  );
});

// The tables are exhaustive over their unions by the compiler; that a listed key
// has WORDS behind it is the half TypeScript cannot see. Without this, a code
// added to the read model and mapped to a key nobody wrote renders as
// `goal.stop.<code>` on screen, in all eight languages.
describe("the wording tables", () => {
  it("names a catalog entry for every stop code", () => {
    const unworded = Object.values(GOAL_STOP_I18N).filter((key) => !(key in en));
    expect(unworded).toEqual([]);
  });

  it("names a catalog entry for every status", () => {
    const unworded = Object.values(GOAL_STATUS_I18N)
      .map((status) => status.label)
      .filter((key) => !(key in en));
    expect(unworded).toEqual([]);
  });
});
