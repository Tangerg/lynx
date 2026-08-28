import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import type { GoalReadModel } from "../application/goalReadModel";
import { GoalStatusSurface } from "./GoalStatusSurface";

const model = vi.hoisted(() => ({
  clearGoal: vi.fn(async () => {}),
  updateGoal: vi.fn(async () => {}),
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
  clearGoal: model.clearGoal,
  updateGoal: model.updateGoal,
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
      reasoningEffort: "",
      createdAt: "2026-08-12T08:00:00Z",
      updatedAt: "2026-08-12T08:01:00Z",
    };
    model.stopGoal.mockClear();
    model.resumeGoal.mockClear();
    model.clearGoal.mockClear();
    model.updateGoal.mockClear();
    model.runtimeAvailable = true;
    model.generation = 1;
  });

  it("pauses an active goal from the standing surface", async () => {
    render(<GoalStatusSurface />);
    fireEvent.click(screen.getByRole("button", { name: "Pause goal" }));
    await vi.waitFor(() => expect(model.stopGoal).toHaveBeenCalledWith("session-a"));
    expect(model.resumeGoal).not.toHaveBeenCalled();
  });

  it("offers Codex Goal management actions in clear, lifecycle, edit order", () => {
    const { container } = render(<GoalStatusSurface />);

    const row = container.querySelector<HTMLElement>('[data-slot="goal-status-row"]');
    expect(row).not.toBeNull();
    expect(row!.className).toContain("py-1");
    expect(screen.getByRole("button", { name: "Pursuing goal Ship alpha" }).className).toContain(
      "leading-[max(1rem,1.2em)]",
    );

    expect(
      Array.from(container.querySelectorAll('[data-slot="goal-actions"] button')).map((button) =>
        button.getAttribute("aria-label"),
      ),
    ).toEqual(["Clear goal", "Pause goal", "Edit goal"]);
  });

  it("opens the same objective editor from the Goal summary", () => {
    render(<GoalStatusSurface />);

    fireEvent.click(screen.getByRole("button", { name: "Pursuing goal Ship alpha" }));

    expect(screen.getByRole("dialog", { name: "Edit goal" })).toBeTruthy();
  });

  it("clears the current goal through the Goal command owner", async () => {
    render(<GoalStatusSurface />);

    fireEvent.click(screen.getByRole("button", { name: "Clear goal" }));

    await vi.waitFor(() => expect(model.clearGoal).toHaveBeenCalledWith("session-a"));
  });

  it("edits only the objective in a Codex-style Goal dialog", async () => {
    render(<GoalStatusSurface />);

    fireEvent.click(screen.getByRole("button", { name: "Edit goal" }));
    const objective = screen.getByRole("textbox", { name: "Goal" });
    expect((objective as HTMLTextAreaElement).value).toBe("Ship alpha");

    fireEvent.change(objective, { target: { value: "  Ship beta  " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await vi.waitFor(() =>
      expect(model.updateGoal).toHaveBeenCalledWith({
        sessionId: "session-a",
        objective: "Ship beta",
      }),
    );
  });

  it("does not submit the Goal editor while an IME composition is active", () => {
    render(<GoalStatusSurface />);

    fireEvent.click(screen.getByRole("button", { name: "Edit goal" }));
    const objective = screen.getByRole("textbox", { name: "Goal" });
    fireEvent.change(objective, { target: { value: "Ship beta" } });
    fireEvent.keyDown(objective, {
      key: "Enter",
      code: "Enter",
      ctrlKey: true,
      isComposing: true,
    });

    expect(model.updateGoal).not.toHaveBeenCalled();
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
