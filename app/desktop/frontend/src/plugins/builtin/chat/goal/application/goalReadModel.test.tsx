import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const material = vi.hoisted(() => ({
  generation: 1,
  value: undefined as GoalState | undefined,
}));

vi.mock("@/plugins/builtin/agent/public/sessionMaterial", () => ({
  useAgentSessionSharedMaterial: () => material,
}));

import { type GoalState, useGoalMaterial } from "./goalReadModel";

const state = (objective: string): GoalState => ({
  available: true,
  goal: {
    sessionId: "ses_goal",
    objective,
    status: "active",
    stop: null,
    budget: { maxRuns: 0, maxCostUsd: 0, maxSteps: 0 },
    used: { runs: 0, costUsd: 0, steps: 0 },
    provider: "openai",
    model: "gpt-5",
    reasoningEffort: "high",
    createdAt: "2026-08-18T00:00:00Z",
    updatedAt: "2026-08-18T00:00:00Z",
  },
});

afterEach(() => {
  material.generation = 1;
  material.value = undefined;
});

describe("useGoalMaterial", () => {
  it("publishes the Goal together with the Agent projection generation that admitted it", () => {
    material.value = state("predecessor");
    const { result, rerender } = renderHook(() => useGoalMaterial());
    expect(result.current).toEqual({ generation: 1, value: state("predecessor") });

    material.generation = 2;
    material.value = state("successor");
    rerender();
    expect(result.current).toEqual({ generation: 2, value: state("successor") });
  });
});
