import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import {
  configureGoalCommandsGateway,
  type GoalCommandsGateway,
} from "./ports/goalCommandsGateway";
import { GOAL_KEY } from "./goalQueries";
import { resumeGoal, startGoal, stopGoal } from "./goalCommands";

let restoreGateway: (() => void) | undefined;

afterEach(() => {
  restoreGateway?.();
  restoreGateway = undefined;
  vi.restoreAllMocks();
});

describe("Goal lifecycle commands", () => {
  it.each([
    {
      name: "start",
      run: () => startGoal({ sessionId: "ses_goal", objective: "ship it" }),
      gateway: { start: vi.fn().mockRejectedValue(new Error("start response lost")) },
    },
    {
      name: "stop",
      run: () => stopGoal("ses_goal"),
      gateway: { stop: vi.fn().mockRejectedValue(new Error("stop response lost")) },
    },
    {
      name: "resume",
      run: () => resumeGoal("ses_goal"),
      gateway: { resume: vi.fn().mockRejectedValue(new Error("resume response lost")) },
    },
  ])("revalidates authoritative Goal state when $name settlement is ambiguous", async (test) => {
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    restoreGateway = configureGoalCommandsGateway({
      start: vi.fn().mockResolvedValue(undefined),
      stop: vi.fn().mockResolvedValue(undefined),
      resume: vi.fn().mockResolvedValue(undefined),
      ...test.gateway,
    } as GoalCommandsGateway);

    await expect(test.run()).rejects.toThrow("response lost");

    expect(invalidate).toHaveBeenCalledWith({
      queryKey: [GOAL_KEY, { sessionId: "ses_goal" }],
    });
  });

  it("preserves the command failure when authoritative revalidation also fails", async () => {
    const commandError = new Error("stop response lost");
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("query unavailable"));
    restoreGateway = configureGoalCommandsGateway({
      start: vi.fn().mockResolvedValue(undefined),
      stop: vi.fn().mockRejectedValue(commandError),
      resume: vi.fn().mockResolvedValue(undefined),
    });

    await expect(stopGoal("ses_goal")).rejects.toBe(commandError);
  });
});
