import { describe, expect, it } from "vitest";
import {
  runtimeGoalMaterial,
  toGoalCommandReceipt,
  toGoalReadModel,
} from "./runtimeGoalCommandsGateway";

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
        reasoningEffort: "high",
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
      reasoningEffort: "high",
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

  it("projects the mounted Goal and clears it when the capability is unavailable", () => {
    const goal = {
      sessionId: "ses_goal",
      objective: "ship it",
      status: "active" as const,
      budget: {},
      used: { runs: 0, costUsd: 0, steps: 0 },
      createdAt: "2026-08-12T08:00:00Z",
      updatedAt: "2026-08-12T08:01:00Z",
    };
    expect(runtimeGoalMaterial(goal, true)).toMatchObject({
      available: true,
      goal: { objective: "ship it" },
    });

    expect(runtimeGoalMaterial(goal, false)).toEqual({ available: false, goal: null });
  });
});
