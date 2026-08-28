import { describe, expect, it } from "vitest";
import { en } from "@/lib/i18n/locales/en";
import { GOAL_STATUS_I18N, goalCanResume } from "./goalStatusPresentation";
import type { GoalReadModel } from "./goalReadModel";

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
    reasoningEffort: "high",
    createdAt: "2026-08-12T08:00:00Z",
    updatedAt: "2026-08-12T08:01:00Z",
    ...patch,
  };
}

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

describe("the wording tables", () => {
  it("names a catalog entry for every status", () => {
    const unworded = Object.values(GOAL_STATUS_I18N)
      .map((status) => status.label)
      .filter((key) => !(key in en));
    expect(unworded).toEqual([]);
  });
});
