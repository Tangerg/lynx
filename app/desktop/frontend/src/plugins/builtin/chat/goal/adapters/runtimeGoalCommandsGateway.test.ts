import { afterEach, describe, expect, it } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { GOAL_KEY } from "../application/goalQueries";
import {
  commitRuntimeGoalMaterial,
  toGoalCommandReceipt,
  toGoalReadModel,
} from "./runtimeGoalCommandsGateway";

afterEach(() => queryClient.clear());

describe("Runtime Goal Adapter", () => {
  it("projects the complete wire Goal into the neutral read model", () => {
    expect(
      toGoalReadModel({
        sessionId: "ses_goal",
        objective: "ship it",
        status: "blocked",
        reason: { code: "blockedByModel", detail: "need an answer" },
        provider: "openai",
        model: "gpt-5",
        budget: { maxRuns: 4, maxCostUsd: 2, maxSteps: 50 },
        used: { runs: 2, costUsd: 0.75, steps: 12 },
        createdAt: "2026-08-12T08:00:00Z",
        updatedAt: "2026-08-12T08:01:00Z",
      }),
    ).toEqual({
      sessionId: "ses_goal",
      objective: "ship it",
      status: "blocked",
      stop: { code: "blockedByModel", detail: "need an answer" },
      provider: "openai",
      model: "gpt-5",
      budget: { maxRuns: 4, maxCostUsd: 2, maxSteps: 50 },
      used: { runs: 2, costUsd: 0.75, steps: 12 },
      createdAt: "2026-08-12T08:00:00Z",
      updatedAt: "2026-08-12T08:01:00Z",
    });
  });

  it("projects a mutation snapshot to a correlation receipt instead of standing state", () => {
    expect(
      toGoalCommandReceipt({
        sessionId: "ses_goal",
      }),
    ).toEqual({ sessionId: "ses_goal" });
  });

  it("commits the mounted material Goal and clears it when the capability is unavailable", () => {
    const goal = {
      sessionId: "ses_goal",
      objective: "ship it",
      status: "active" as const,
      budget: {},
      used: { runs: 0, costUsd: 0, steps: 0 },
      createdAt: "2026-08-12T08:00:00Z",
      updatedAt: "2026-08-12T08:01:00Z",
    };
    const key = [GOAL_KEY, { sessionId: "ses_goal" }] as const;

    commitRuntimeGoalMaterial("ses_goal", goal, true);
    expect(queryClient.getQueryData(key)).toMatchObject({
      available: true,
      goal: { objective: "ship it" },
    });

    commitRuntimeGoalMaterial("ses_goal", goal, false);
    expect(queryClient.getQueryData(key)).toEqual({ available: false, goal: null });
  });
});
