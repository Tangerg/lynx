import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { GoalReadModel } from "../application/goalQueries";
import { GoalBanner } from "./GoalBanner";

const model = vi.hoisted(() => ({
  sessionId: "session-a",
  stopGoal: vi.fn(async () => {}),
  resumeGoal: vi.fn(async () => {}),
  goal: {
    sessionId: "session-a",
    objective: "Ship alpha",
    status: "active",
    stop: null,
    budget: { maxRuns: 10, maxCostUsd: 0, maxSteps: 0 },
    used: { runs: 1, costUsd: 0, steps: 2 },
  } as GoalReadModel,
}));

vi.mock("../application/goalCommands", () => ({
  stopGoal: model.stopGoal,
  resumeGoal: model.resumeGoal,
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionId: () => model.sessionId,
}));

vi.mock("../application/goalQueries", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../application/goalQueries")>()),
  useGoal: () => ({ data: { available: true, goal: model.goal } }),
}));

describe("GoalBanner disclosure identity", () => {
  beforeEach(() => {
    model.sessionId = "session-a";
    model.goal = {
      sessionId: "session-a",
      objective: "Ship alpha",
      status: "active",
      stop: null,
      budget: { maxRuns: 10, maxCostUsd: 0, maxSteps: 0 },
      used: { runs: 1, costUsd: 0, steps: 2 },
    };
    model.stopGoal.mockClear();
    model.resumeGoal.mockClear();
  });

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

  it("stops an active goal from the standing surface", async () => {
    render(<GoalBanner />);
    fireEvent.click(screen.getByRole("button", { name: "Stop" }));
    await vi.waitFor(() => expect(model.stopGoal).toHaveBeenCalledWith("session-a"));
    expect(model.resumeGoal).not.toHaveBeenCalled();
  });

  it("resumes a paused goal from the standing surface", async () => {
    model.goal = { ...model.goal, status: "paused" };
    render(<GoalBanner />);
    fireEvent.click(screen.getByRole("button", { name: "Resume" }));
    await vi.waitFor(() => expect(model.resumeGoal).toHaveBeenCalledWith("session-a"));
    expect(model.stopGoal).not.toHaveBeenCalled();
  });

  it("keeps a completing goal visible without exposing a lifecycle command", () => {
    model.goal = { ...model.goal, status: "completing" };
    render(<GoalBanner />);

    expect(screen.getByText("Finishing")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Stop" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Resume" })).toBeNull();
    expect(model.stopGoal).not.toHaveBeenCalled();
    expect(model.resumeGoal).not.toHaveBeenCalled();
  });
});
