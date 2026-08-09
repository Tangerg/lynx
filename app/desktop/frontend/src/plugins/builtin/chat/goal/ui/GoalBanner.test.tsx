import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { GoalReadModel } from "../application/goalQueries";
import { GoalBanner } from "./GoalBanner";

const model = vi.hoisted(() => ({
  sessionId: "session-a",
  goal: {
    sessionId: "session-a",
    objective: "Ship alpha",
    status: "active",
    stop: null,
    budget: { maxRuns: 10, maxCostUsd: 0, maxSteps: 0 },
    used: { runs: 1, costUsd: 0, steps: 2 },
  } as GoalReadModel,
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionId: () => model.sessionId,
}));

vi.mock("../application/goalQueries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../application/goalQueries")>()),
  useGoal: () => ({ data: { available: true, goal: model.goal } }),
}));

describe("GoalBanner disclosure identity", () => {
  it("preserves a choice while a goal advances and resets it for another goal", () => {
    const { rerender } = render(<GoalBanner />);
    fireEvent.click(screen.getByRole("button", { name: "Show the allowance" }));
    expect(
      screen.getByRole("button", { name: "Hide the allowance" }).getAttribute("aria-expanded"),
    ).toBe("true");

    model.goal = { ...model.goal, used: { ...model.goal.used, runs: 2 } };
    rerender(<GoalBanner />);
    expect(
      screen.getByRole("button", { name: "Hide the allowance" }).getAttribute("aria-expanded"),
    ).toBe("true");

    model.sessionId = "session-b";
    model.goal = { ...model.goal, sessionId: "session-b", objective: "Ship beta" };
    rerender(<GoalBanner />);
    expect(
      screen.getByRole("button", { name: "Show the allowance" }).getAttribute("aria-expanded"),
    ).toBe("false");
  });
});
