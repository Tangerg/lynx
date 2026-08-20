import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { GoalComposerModeOwner } from "../application/goalComposerMode";
import { GoalModeIndicator } from "./GoalModeIndicator";

const model = vi.hoisted(() => ({
  sessionId: "session-a",
  goal: null as { status: "active" | "paused" | "blocked" | "completing" } | null,
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionId: () => model.sessionId,
}));

vi.mock("../application/goalReadModel", () => ({
  useGoalMaterial: () => ({ value: { available: true, goal: model.goal } }),
}));

let owner: GoalComposerModeOwner;

beforeEach(() => {
  model.sessionId = "session-a";
  model.goal = null;
  owner = GoalComposerModeOwner.install();
});

afterEach(async () => {
  cleanup();
  owner.dispose();
  await drainBrowserTasks();
});

describe("GoalModeIndicator", () => {
  it("renders no permanent Goal launcher while composer mode is inactive", () => {
    render(<GoalModeIndicator />);

    expect(screen.queryByText("Goal")).toBeNull();
    expect(screen.queryAllByRole("spinbutton")).toHaveLength(0);
  });

  it("renders the compact Codex-style mode indicator and clears it in place", () => {
    owner.activate("session-a");
    render(<GoalModeIndicator />);

    const indicator = screen.getByRole("button", { name: "Exit Goal mode" });
    expect(indicator.getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByText("Goal")).toBeTruthy();
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);
    expect(screen.queryAllByRole("spinbutton")).toHaveLength(0);

    fireEvent.click(indicator);
    expect(screen.queryByText("Goal")).toBeNull();
  });

  it("keeps replacement confirmation separate from the composer draft", () => {
    owner.activate("session-a");
    owner.requestReplacement("session-a", "old objective", vi.fn());
    render(<GoalModeIndicator />);

    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByText(/old objective/)).toBeTruthy();
    expect(screen.queryAllByRole("textbox")).toHaveLength(0);
    expect(screen.queryAllByRole("spinbutton")).toHaveLength(0);

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(owner.snapshot().phase).toBe("armed");
  });

  it("keeps the starting owner until its command settles when standing material arrives first", () => {
    owner.activate("session-a");
    owner.begin("session-a");
    const { rerender } = render(<GoalModeIndicator />);

    model.goal = { status: "active" };
    rerender(<GoalModeIndicator />);

    expect(owner.snapshot()).toMatchObject({ sessionId: "session-a", phase: "starting" });
  });
});
