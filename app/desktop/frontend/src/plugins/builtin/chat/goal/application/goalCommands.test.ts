import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryObserver } from "@tanstack/react-query";
import { queryClient } from "@/lib/queryClient";
import { type GoalCommandReceipt, type GoalCommandsGateway } from "./ports/goalCommandsGateway";
import { GOAL_KEY, type GoalState } from "./goalQueries";
import {
  GoalCommandOwner,
  GoalCommandSessionMismatchError,
  goalCommandWasRetired,
  resumeGoal,
  startGoal,
  stopGoal,
} from "./goalCommands";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((onResolve) => {
    resolve = onResolve;
  });
  return { promise, resolve };
}

let restoreGateway: (() => void) | undefined;

function installGoalCommandOwnerForTest(gateway: GoalCommandsGateway): () => void {
  const owner = GoalCommandOwner.install(gateway);
  return () => owner.dispose();
}

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

const receipt = { sessionId: "ses_goal" };

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
    restoreGateway = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop: vi.fn().mockResolvedValue(receipt),
      resume: vi.fn().mockResolvedValue(receipt),
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
    restoreGateway = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop: vi.fn().mockRejectedValue(commandError),
      resume: vi.fn().mockResolvedValue(receipt),
    });

    await expect(stopGoal("ses_goal")).rejects.toBe(commandError);
  });

  it("does not turn an accepted Goal command into a failure when projection repair fails", async () => {
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("query unavailable"));
    restoreGateway = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop: vi.fn().mockResolvedValue(receipt),
      resume: vi.fn().mockResolvedValue(receipt),
    });

    await expect(stopGoal("ses_goal")).resolves.toBeUndefined();
  });

  it("keeps goals.get as the sole author of the standing read model", async () => {
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    queryClient.setQueryData([GOAL_KEY, { sessionId: "ses_goal" }], {
      available: true,
      goal: { ...goal, used: { ...goal.used, runs: 2 } },
    });
    restoreGateway = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop: vi.fn().mockResolvedValue(receipt),
      resume: vi.fn().mockResolvedValue(receipt),
    });

    await stopGoal("ses_goal");

    expect(queryClient.getQueryData([GOAL_KEY, { sessionId: "ses_goal" }])).toEqual({
      available: true,
      goal: { ...goal, used: { ...goal.used, runs: 2 } },
    });
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: [GOAL_KEY, { sessionId: "ses_goal" }],
      exact: true,
    });
  });

  it("serializes lifecycle commands for one Session while leaving intent order intact", async () => {
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    const stopping = deferred<GoalCommandReceipt>();
    const stop = vi.fn(() => stopping.promise);
    const resume = vi.fn().mockResolvedValue(receipt);
    restoreGateway = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop,
      resume,
    });

    const first = stopGoal("ses_goal");
    const second = resumeGoal("ses_goal");
    await vi.waitFor(() => expect(stop).toHaveBeenCalledOnce());
    expect(resume).not.toHaveBeenCalled();

    stopping.resolve({ sessionId: "ses_goal" });
    await expect(first).resolves.toBeUndefined();
    await expect(second).resolves.toBeUndefined();
    expect(resume).toHaveBeenCalledOnce();
  });

  it("keeps the next lifecycle command behind authoritative revalidation", async () => {
    const revalidated = deferred<void>();
    vi.spyOn(queryClient, "invalidateQueries").mockReturnValue(revalidated.promise);
    const stop = vi.fn().mockResolvedValue(receipt);
    const resume = vi.fn().mockResolvedValue(receipt);
    restoreGateway = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
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

  it("retires in-flight and queued Goal commands when their owner is replaced", async () => {
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    const stopping = deferred<GoalCommandReceipt>();
    const retiredStop = vi.fn(() => stopping.promise);
    const retiredResume = vi.fn().mockResolvedValue(receipt);
    const disposePredecessor = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop: retiredStop,
      resume: retiredResume,
    });
    restoreGateway = disposePredecessor;

    const inFlight = stopGoal("ses_goal");
    const queued = resumeGoal("ses_goal");
    let inFlightOutcome = "pending";
    let queuedOutcome = "pending";
    const observedInFlight = inFlight.then(
      () => {
        inFlightOutcome = "resolved";
      },
      () => {
        inFlightOutcome = "retired";
      },
    );
    const observedQueued = queued.then(
      () => {
        queuedOutcome = "resolved";
      },
      () => {
        queuedOutcome = "retired";
      },
    );
    await vi.waitFor(() => expect(retiredStop).toHaveBeenCalledOnce());

    const successorResume = vi.fn().mockResolvedValue(receipt);
    const disposeSuccessor = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop: vi.fn().mockResolvedValue(receipt),
      resume: successorResume,
    });
    restoreGateway = () => {
      disposeSuccessor();
      disposePredecessor();
    };
    await vi.waitFor(() => expect(inFlightOutcome).toBe("retired"));
    await vi.waitFor(() => expect(queuedOutcome).toBe("retired"));
    const outcomesAtReplacement = [inFlightOutcome, queuedOutcome];

    stopping.resolve(receipt);
    await Promise.all([observedInFlight, observedQueued]);

    expect(outcomesAtReplacement).toEqual(["retired", "retired"]);
    expect(retiredResume).not.toHaveBeenCalled();
    expect(successorResume).not.toHaveBeenCalled();
  });

  it("gives a Runtime successor a new gateway without lending it queued predecessor intents", async () => {
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    const stopping = deferred<GoalCommandReceipt>();
    const retiredStop = vi.fn(() => stopping.promise);
    const retiredResume = vi.fn().mockResolvedValue(receipt);
    const owner = GoalCommandOwner.install({
      start: vi.fn().mockResolvedValue(receipt),
      stop: retiredStop,
      resume: retiredResume,
    });
    restoreGateway = () => owner.dispose();

    const inFlight = stopGoal("ses_goal").catch((error: unknown) => error);
    const queued = resumeGoal("ses_goal").catch((error: unknown) => error);
    await vi.waitFor(() => expect(retiredStop).toHaveBeenCalledOnce());

    const successorResume = vi.fn().mockResolvedValue(receipt);
    expect(
      owner.replaceRuntimeGeneration({
        start: vi.fn().mockResolvedValue(receipt),
        stop: vi.fn().mockResolvedValue(receipt),
        resume: successorResume,
      }),
    ).toBe(true);

    expect(goalCommandWasRetired(await inFlight)).toBe(true);
    expect(goalCommandWasRetired(await queued)).toBe(true);
    expect(retiredResume).not.toHaveBeenCalled();
    expect(successorResume).not.toHaveBeenCalled();

    await expect(resumeGoal("ses_goal")).resolves.toBeUndefined();
    expect(successorResume).toHaveBeenCalledOnce();
    stopping.resolve(receipt);
  });

  it("does not let a delayed command response regress newer autonomous state", async () => {
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    queryClient.setQueryData([GOAL_KEY, { sessionId: "ses_goal" }], {
      available: true,
      goal: { ...goal, used: { ...goal.used, runs: 2 } },
    });
    restoreGateway = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop: vi.fn().mockResolvedValue(receipt),
      resume: vi.fn().mockResolvedValue(receipt),
    });

    await stopGoal("ses_goal");

    expect(queryClient.getQueryData([GOAL_KEY, { sessionId: "ses_goal" }])).toMatchObject({
      goal: { status: "active", used: { runs: 2 }, updatedAt: goal.updatedAt },
    });
  });

  it("settles a mounted Goal query from its authoritative fetcher after success", async () => {
    const queryKey = [GOAL_KEY, { sessionId: "ses_goal" }] as const;
    let authoritative: GoalState = { available: true, goal };
    const observer = new QueryObserver(queryClient, {
      queryKey,
      queryFn: async () => authoritative,
      experimental_prefetchInRender: true,
    });
    const unsubscribe = observer.subscribe(() => undefined);
    try {
      await observer.refetch();
      authoritative = {
        available: true,
        goal: {
          ...goal,
          status: "paused",
          stop: { code: "stoppedByUser", detail: "" },
          updatedAt: "2026-08-12T08:01:00Z",
        },
      };
      restoreGateway = installGoalCommandOwnerForTest({
        start: vi.fn().mockResolvedValue(receipt),
        stop: vi.fn().mockResolvedValue(receipt),
        resume: vi.fn().mockResolvedValue(receipt),
      });

      await stopGoal("ses_goal");

      expect(queryClient.getQueryData(queryKey)).toEqual(authoritative);
    } finally {
      unsubscribe();
    }
  });

  it("rejects a mutation response addressed to another Session and still revalidates", async () => {
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    restoreGateway = installGoalCommandOwnerForTest({
      start: vi.fn().mockResolvedValue(receipt),
      stop: vi.fn().mockResolvedValue({ sessionId: "ses_other" }),
      resume: vi.fn().mockResolvedValue(receipt),
    });

    const failure = await stopGoal("ses_goal").catch((error: unknown) => error);

    expect(failure).toBeInstanceOf(GoalCommandSessionMismatchError);
    expect(failure).toMatchObject({
      expectedSessionId: "ses_goal",
      actualSessionId: "ses_other",
    });

    expect(invalidate).toHaveBeenCalledWith({
      queryKey: [GOAL_KEY, { sessionId: "ses_goal" }],
      exact: true,
    });
  });
});
