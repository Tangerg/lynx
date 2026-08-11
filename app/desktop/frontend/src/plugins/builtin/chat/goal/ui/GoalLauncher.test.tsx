import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { GoalLauncher } from "./GoalLauncher";

const model = vi.hoisted(() => ({
  composerText: "Ship alpha",
  setComposerText: vi.fn(),
  startGoal: vi.fn(async () => {}),
}));

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionId: () => "session-a",
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

vi.mock("../application/goalQueries", () => ({
  useGoal: () => ({ data: { available: true, goal: null } }),
}));

vi.mock("../application/goalCommands", () => ({
  startGoal: model.startGoal,
}));

describe("GoalLauncher", () => {
  beforeEach(() => {
    model.composerText = "Ship alpha";
    model.setComposerText.mockClear();
    model.startGoal.mockClear();
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
});
