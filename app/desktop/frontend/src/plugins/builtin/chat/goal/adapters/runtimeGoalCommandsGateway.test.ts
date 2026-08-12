import { describe, expect, it } from "vitest";
import { toGoalReadModel } from "./runtimeGoalCommandsGateway";

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
});
