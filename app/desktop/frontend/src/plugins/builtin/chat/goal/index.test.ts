import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import type { Goal, LyraClient, MutationPromise } from "@/rpc";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";
import { resumeGoal, stopGoal } from "./application/goalCommands";
import { GOAL_KEY } from "./application/goalQueries";
import goalPlugin from "./index";

afterEach(async () => {
  await resetKernelForTest();
  resetContainer();
  queryClient.removeQueries({ queryKey: [GOAL_KEY] });
  vi.restoreAllMocks();
});

describe("Goal plugin Runtime generation wiring", () => {
  it("retires predecessor commands and binds successor commands to the successor client", async () => {
    const retired = deferred<Goal>();
    const retiredStop = vi.fn(() => mutation(retired.promise, "retired-stop"));
    setContainer({
      client: () => ({ goals: { stop: retiredStop } }) as unknown as LyraClient,
    });
    let generation = "runtime_1";
    const subscribers = new Set<() => void>();
    const runtime = definePlugin({
      name: "test.goal-runtime-generation",
      provides: { stream: RUNTIME_STREAM_PORTS },
      setup() {
        return {
          stream: {
            connectionGeneration: () => generation,
            subscribeConnection(onChange: () => void) {
              subscribers.add(onChange);
              return () => subscribers.delete(onChange);
            },
            reportConnectionLoss: vi.fn(),
          },
        };
      },
    });
    await loadPluginsForTest(runtime, goalPlugin);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();

    const predecessor = rejected(stopGoal("ses_goal"));
    await vi.waitFor(() => expect(retiredStop).toHaveBeenCalledOnce());

    const successorResume = vi.fn(() =>
      mutation(Promise.resolve(runtimeGoal("ses_goal")), "successor-resume"),
    );
    setContainer({
      client: () => ({ goals: { resume: successorResume } }) as unknown as LyraClient,
    });
    generation = "runtime_2";
    for (const subscriber of subscribers) subscriber();

    await expect(predecessor).resolves.toMatchObject({
      message: "goal_command_generation_retired",
    });
    await expect(resumeGoal("ses_goal")).resolves.toBeUndefined();
    expect(successorResume).toHaveBeenCalledOnce();
    expect(invalidate).toHaveBeenCalledOnce();

    retired.resolve(runtimeGoal("ses_goal"));
    await Promise.resolve();
    await Promise.resolve();
    expect(invalidate).toHaveBeenCalledOnce();
  });
});

function mutation<T>(promise: Promise<T>, idempotencyKey: string): MutationPromise<T> {
  return Object.assign(promise, {
    idempotencyKey,
    retry: vi.fn(),
  });
}

function runtimeGoal(sessionId: string): Goal {
  return {
    sessionId,
    objective: "Keep one generation",
    status: "active",
    budget: {},
    used: { runs: 0, costUsd: 0, steps: 0 },
    createdAt: "2026-08-18T00:00:00Z",
    updatedAt: "2026-08-18T00:00:00Z",
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((settle) => {
    resolve = settle;
  });
  return { promise, resolve };
}

function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}
