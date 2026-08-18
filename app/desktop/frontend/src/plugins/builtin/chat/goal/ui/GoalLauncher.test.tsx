import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { GoalLauncher } from "./GoalLauncher";

const model = vi.hoisted(() => ({
  sessionId: "session-a",
  composerText: "Ship alpha",
  goal: null as { status: "active" | "paused" | "blocked" | "completing" } | null,
  setComposerText: vi.fn(),
  startGoal: vi.fn(async () => {}),
  runtimeAvailable: true,
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  getActiveSessionId: () => model.sessionId,
  useActiveSessionId: () => model.sessionId,
}));

vi.mock("@/plugins/builtin/chat/composer/public/draft", () => ({
  useComposerText: () => model.composerText,
  useSetComposerText: () => model.setComposerText,
  getComposerText: () => model.composerText,
}));

vi.mock("@/plugins/builtin/chat/composer/public/modelPreference", () => ({
  useComposerModelPreference: () => ({ provider: "openai", model: "gpt-5" }),
}));

vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  useRuntimeCapability: () => true,
}));

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => model.runtimeAvailable,
  useRuntimeCommandsAvailable: () => model.runtimeAvailable,
}));

vi.mock("../application/goalReadModel", () => ({
  useGoalMaterial: () => ({
    generation: 1,
    value: { available: true, goal: model.goal },
  }),
}));

vi.mock("../application/goalCommands", () => ({
  startGoal: model.startGoal,
  goalCommandWasRetired: () => false,
}));

describe("GoalLauncher", () => {
  afterEach(async () => {
    cleanup();
    await drainBrowserTasks();
  });

  beforeEach(() => {
    model.sessionId = "session-a";
    model.composerText = "Ship alpha";
    model.goal = null;
    model.setComposerText.mockClear();
    model.startGoal.mockClear();
    model.runtimeAvailable = true;
  });

  it("starts an uncapped goal from the current draft and consumes only that text", async () => {
    render(<GoalLauncher />);
    fireEvent.click(screen.getByRole("button", { name: "Start Goal" }));
    fireEvent.click(screen.getAllByRole("button", { name: "Start Goal" }).at(-1)!);

    await vi.waitFor(() =>
      expect(model.startGoal).toHaveBeenCalledWith({
        sessionId: "session-a",
        objective: "Ship alpha",
        provider: "openai",
        model: "gpt-5",
      }),
    );
    expect(model.setComposerText).toHaveBeenCalledWith("");
  });

  it("keeps text edited while the goal request is in flight", async () => {
    let finish!: () => void;
    model.startGoal.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    render(<GoalLauncher />);
    fireEvent.click(screen.getByRole("button", { name: "Start Goal" }));
    fireEvent.click(screen.getAllByRole("button", { name: "Start Goal" }).at(-1)!);
    model.composerText = "A newer draft";
    finish();

    await vi.waitFor(() => expect(model.startGoal).toHaveBeenCalledOnce());
    expect(model.setComposerText).not.toHaveBeenCalled();
  });

  it("does not open or start a Goal while Runtime is offline", () => {
    model.runtimeAvailable = false;
    render(<GoalLauncher />);

    const start = screen.getByRole("button", { name: "Start Goal" });
    expect(start.hasAttribute("disabled")).toBe(true);
    fireEvent.click(start);
    expect(screen.queryByRole("textbox", { name: "Objective" })).toBeNull();
    expect(model.startGoal).not.toHaveBeenCalled();
  });

  it.each(["paused", "blocked"] as const)(
    "keeps goals.start reachable so a %s goal can be replaced",
    (status) => {
      model.goal = { status };
      render(<GoalLauncher />);

      expect(screen.getByRole("button", { name: "Start Goal" })).toBeTruthy();
    },
  );

  it.each(["active", "completing"] as const)(
    "does not offer a conflicting start while a goal is %s",
    (status) => {
      model.goal = { status };
      render(<GoalLauncher />);

      expect(screen.queryByRole("button", { name: "Start Goal" })).toBeNull();
    },
  );

  it("resets the open draft when active Session identity changes", () => {
    const { rerender } = render(<GoalLauncher />);
    fireEvent.click(screen.getByRole("button", { name: "Start Goal" }));
    expect((screen.getByRole("textbox", { name: "Objective" }) as HTMLTextAreaElement).value).toBe(
      "Ship alpha",
    );

    model.sessionId = "session-b";
    model.composerText = "Ship beta";
    rerender(<GoalLauncher />);

    expect(screen.queryByRole("textbox", { name: "Objective" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Start Goal" }));
    expect((screen.getByRole("textbox", { name: "Objective" }) as HTMLTextAreaElement).value).toBe(
      "Ship beta",
    );
  });

  it("does not consume the next Session's matching draft after an old start settles", async () => {
    let finish!: () => void;
    model.startGoal.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    const { rerender } = render(<GoalLauncher />);
    fireEvent.click(screen.getByRole("button", { name: "Start Goal" }));
    fireEvent.click(screen.getAllByRole("button", { name: "Start Goal" }).at(-1)!);

    model.sessionId = "session-b";
    rerender(<GoalLauncher />);
    finish();

    await vi.waitFor(() => expect(model.startGoal).toHaveBeenCalledOnce());
    expect(model.setComposerText).not.toHaveBeenCalled();
  });
});
