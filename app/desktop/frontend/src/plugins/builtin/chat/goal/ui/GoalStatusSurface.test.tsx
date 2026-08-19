import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import type { GoalReadModel } from "../application/goalReadModel";
import { GoalStatusSurface } from "./GoalStatusSurface";

const model = vi.hoisted(() => ({
  stopGoal: vi.fn(async () => {}),
  resumeGoal: vi.fn(async () => {}),
  runtimeAvailable: true,
  generation: 1,
  goal: {
    sessionId: "session-a",
    objective: "Ship alpha",
    status: "active",
    stop: null,
    budget: { maxRuns: 10, maxCostUsd: 0, maxSteps: 0 },
    used: { runs: 1, costUsd: 0, steps: 2 },
    provider: "openai",
    model: "gpt-5",
    createdAt: "2026-08-12T08:00:00Z",
    updatedAt: "2026-08-12T08:01:00Z",
  } as GoalReadModel,
}));

vi.mock("../application/goalCommands", () => ({
  stopGoal: model.stopGoal,
  resumeGoal: model.resumeGoal,
  goalCommandWasRetired: () => false,
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => model.runtimeAvailable,
  useRuntimeCommandsAvailable: () => model.runtimeAvailable,
}));

vi.mock("../application/goalReadModel", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../application/goalReadModel")>()),
  useGoalMaterial: () => ({
    generation: model.generation,
    value: { available: true, goal: model.goal },
  }),
}));

describe("Goal status surface", () => {
  afterEach(async () => {
    cleanup();
    await drainBrowserTasks();
  });

  beforeEach(() => {
    model.goal = {
      sessionId: "session-a",
      objective: "Ship alpha",
      status: "active",
      stop: null,
      budget: { maxRuns: 10, maxCostUsd: 0, maxSteps: 0 },
      used: { runs: 1, costUsd: 0, steps: 2 },
      provider: "openai",
      model: "gpt-5",
      createdAt: "2026-08-12T08:00:00Z",
      updatedAt: "2026-08-12T08:01:00Z",
    };
    model.stopGoal.mockClear();
    model.resumeGoal.mockClear();
    model.runtimeAvailable = true;
    model.generation = 1;
  });

  it("pauses an active goal from the standing surface", async () => {
    render(<GoalStatusSurface />);
    fireEvent.click(screen.getByRole("button", { name: "Pause goal" }));
    await vi.waitFor(() => expect(model.stopGoal).toHaveBeenCalledWith("session-a"));
    expect(model.resumeGoal).not.toHaveBeenCalled();
  });

  it("resumes a paused goal from the standing surface", async () => {
    model.goal = { ...model.goal, status: "paused" };
    render(<GoalStatusSurface />);
    fireEvent.click(screen.getByRole("button", { name: "Resume goal" }));
    await vi.waitFor(() => expect(model.resumeGoal).toHaveBeenCalledWith("session-a"));
    expect(model.stopGoal).not.toHaveBeenCalled();
  });

  it("keeps lifecycle commands visible but inert while Runtime is offline", () => {
    model.runtimeAvailable = false;
    render(<GoalStatusSurface />);

    const pause = screen.getByRole("button", { name: "Pause goal" });
    expect(pause.hasAttribute("disabled")).toBe(true);
    fireEvent.click(pause);
    expect(model.stopGoal).not.toHaveBeenCalled();
  });

  it("keeps a completing goal visible without exposing a lifecycle command", () => {
    model.goal = { ...model.goal, status: "completing" };
    render(<GoalStatusSurface />);

    expect(screen.getByText("Finishing goal")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Stop goal" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Resume goal" })).toBeNull();
    expect(model.stopGoal).not.toHaveBeenCalled();
    expect(model.resumeGoal).not.toHaveBeenCalled();
  });

  it("does not offer Resume after a durable budget boundary", () => {
    model.goal = {
      ...model.goal,
      status: "blocked",
      stop: { code: "runBudgetReached", detail: "" },
    };
    render(<GoalStatusSurface />);

    expect(screen.queryByRole("button", { name: "Resume goal" })).toBeNull();
    expect(screen.queryByText("Out of turns")).toBeNull();
  });

  it("presents only Goal status and objective, never limits or usage", () => {
    render(<GoalStatusSurface />);

    expect(screen.getByText("Pursuing goal")).toBeTruthy();
    expect(screen.getByText("Ship alpha")).toBeTruthy();
    expect(screen.queryByText("1/10")).toBeNull();
    expect(screen.queryByText("Turns")).toBeNull();
    expect(screen.queryByText("Steps")).toBeNull();
    expect(screen.queryByRole("button", { name: "Show the allowance" })).toBeNull();
  });

  it("uses the Codex top-tray surface and dedicated Goal glyph", () => {
    const { container } = render(<GoalStatusSurface />);

    const surface = container.querySelector<HTMLElement>('[data-slot="composer-top-tray-surface"]');
    expect(surface).not.toBeNull();
    expect(surface?.className).toContain("rounded-t-composer");
    expect(surface?.className).toContain("border-x");
    expect(surface?.className).toContain("border-t");
    expect(surface?.className).not.toContain("mb-2");
    expect(surface?.querySelector('[data-slot="goal-glyph"]')).not.toBeNull();
  });
});
