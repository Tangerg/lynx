import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import {
  configureGoalCommandsGateway,
  type GoalCommandsGateway,
} from "./ports/goalCommandsGateway";
import { GOAL_KEY, type GoalReadModel } from "./goalQueries";
import { resumeGoal, startGoal, stopGoal } from "./goalCommands";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((onResolve) => {
    resolve = onResolve;
  });
  return { promise, resolve };
}

let restoreGateway: (() => void) | undefined;

afterEach(() => {
  restoreGateway?.();
  restoreGateway = undefined;
  vi.restoreAllMocks();
  queryClient.removeQueries({ queryKey: [GOAL_KEY] });
});

const goal = {
  sessionId: "ses_goal",
  objective: "ship it",
  status: "active" as const,
  stop: null,
  budget: { maxRuns: 3, maxCostUsd: 1, maxSteps: 20 },
  used: { runs: 0, costUsd: 0, steps: 0 },
  provider: "openai",
  model: "gpt-5",
  createdAt: "2026-08-12T08:00:00Z",
  updatedAt: "2026-08-12T08:00:00Z",
};

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
      start: vi.fn().mockResolvedValue(goal),
      stop: vi.fn().mockResolvedValue(goal),
      resume: vi.fn().mockResolvedValue(goal),
      ...test.gateway,
    } as GoalCommandsGateway);

    await expect(test.run()).rejects.toThrow("response lost");

    expect(invalidate).toHaveBeenCalledWith({
      queryKey: [GOAL_KEY, { sessionId: "ses_goal" }],
      exact: true,
    });
  });

  it("preserves the command failure when authoritative revalidation also fails", async () => {
    const commandError = new Error("stop response lost");
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("query unavailable"));
    restoreGateway = configureGoalCommandsGateway({
      start: vi.fn().mockResolvedValue(goal),
      stop: vi.fn().mockRejectedValue(commandError),
      resume: vi.fn().mockResolvedValue(goal),
    });

    await expect(stopGoal("ses_goal")).rejects.toBe(commandError);
  });

  it("commits the typed command response before revalidating autonomous progress", async () => {
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    restoreGateway = configureGoalCommandsGateway({
      start: vi.fn().mockResolvedValue(goal),
      stop: vi.fn().mockResolvedValue({
        ...goal,
        status: "paused",
        stop: { code: "stoppedByUser", detail: "" },
        updatedAt: "2026-08-12T08:01:00Z",
      }),
      resume: vi.fn().mockResolvedValue(goal),
    });

    await stopGoal("ses_goal");

    expect(queryClient.getQueryData([GOAL_KEY, { sessionId: "ses_goal" }])).toEqual({
      available: true,
      goal: expect.objectContaining({ status: "paused", updatedAt: "2026-08-12T08:01:00Z" }),
    });
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: [GOAL_KEY, { sessionId: "ses_goal" }],
      exact: true,
    });
  });

  it("serializes lifecycle commands for one Session while leaving intent order intact", async () => {
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    const stopping = deferred<GoalReadModel>();
    const stop = vi.fn(() => stopping.promise);
    const resume = vi.fn().mockResolvedValue(goal);
    restoreGateway = configureGoalCommandsGateway({
      start: vi.fn().mockResolvedValue(goal),
      stop,
      resume,
    });

    const first = stopGoal("ses_goal");
    const second = resumeGoal("ses_goal");
    await Promise.resolve();
    expect(stop).toHaveBeenCalledOnce();
    expect(resume).not.toHaveBeenCalled();

    stopping.resolve({
      ...goal,
      status: "paused",
      stop: { code: "stoppedByUser", detail: "" },
      updatedAt: "2026-08-12T08:01:00Z",
    });
    await expect(first).resolves.toBeUndefined();
    await expect(second).resolves.toBeUndefined();
    expect(resume).toHaveBeenCalledOnce();
  });

  it("keeps the next lifecycle command behind authoritative revalidation", async () => {
    const revalidated = deferred<void>();
    vi.spyOn(queryClient, "invalidateQueries").mockReturnValue(revalidated.promise);
    const stop = vi.fn().mockResolvedValue({
      ...goal,
      status: "paused",
      stop: { code: "stoppedByUser", detail: "" },
      updatedAt: "2026-08-12T08:01:00Z",
    });
    const resume = vi.fn().mockResolvedValue(goal);
    restoreGateway = configureGoalCommandsGateway({
      start: vi.fn().mockResolvedValue(goal),
      stop,
      resume,
    });

    const first = stopGoal("ses_goal");
    const second = resumeGoal("ses_goal");
    await vi.waitFor(() => expect(stop).toHaveBeenCalledOnce());
    expect(resume).not.toHaveBeenCalled();

    revalidated.resolve();
    await expect(first).resolves.toBeUndefined();
    await expect(second).resolves.toBeUndefined();
    expect(resume).toHaveBeenCalledOnce();
  });

  it("does not let an older command response regress newer autonomous state", async () => {
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    queryClient.setQueryData([GOAL_KEY, { sessionId: "ses_goal" }], {
      available: true,
      goal: { ...goal, used: { ...goal.used, runs: 2 }, updatedAt: "2026-08-12T08:03:00Z" },
    });
    restoreGateway = configureGoalCommandsGateway({
      start: vi.fn().mockResolvedValue(goal),
      stop: vi.fn().mockResolvedValue({
        ...goal,
        status: "paused",
        stop: { code: "stoppedByUser", detail: "" },
        updatedAt: "2026-08-12T08:02:00Z",
      }),
      resume: vi.fn().mockResolvedValue(goal),
    });

    await stopGoal("ses_goal");

    expect(queryClient.getQueryData([GOAL_KEY, { sessionId: "ses_goal" }])).toMatchObject({
      goal: { status: "active", used: { runs: 2 }, updatedAt: "2026-08-12T08:03:00Z" },
    });
  });

  it("orders Runtime timestamps by instant rather than RFC3339 spelling", async () => {
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    queryClient.setQueryData([GOAL_KEY, { sessionId: "ses_goal" }], {
      available: true,
      goal: { ...goal, updatedAt: "2026-08-12T08:00:00Z" },
    });
    restoreGateway = configureGoalCommandsGateway({
      start: vi.fn().mockResolvedValue(goal),
      stop: vi.fn().mockResolvedValue({
        ...goal,
        status: "paused",
        stop: { code: "stoppedByUser", detail: "" },
        updatedAt: "2026-08-12T08:00:00.000000001Z",
      }),
      resume: vi.fn().mockResolvedValue(goal),
    });

    await stopGoal("ses_goal");

    expect(queryClient.getQueryData([GOAL_KEY, { sessionId: "ses_goal" }])).toMatchObject({
      goal: { status: "paused", updatedAt: "2026-08-12T08:00:00.000000001Z" },
    });
  });
});
