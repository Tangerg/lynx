import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  runtimeCapability: () => true,
}));

import { resetContainer, setContainer } from "@/main/container";
import { RpcTransportError, type Goal, type LyraClient, type MutationPromise } from "@/rpc";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import type { Contributor } from "@/plugins/sdk";
import { GOAL_KEY, type GoalQuery, type GoalState } from "../application/goalQueries";
import {
  installGoalRuntimeAdapter,
  type GoalRuntimeAdapterInstallation,
} from "./runtimeGoalCommandsGateway";
import { contributeForTest } from "@/plugins/sdk/testKernel";
import { startGoal } from "../application/goalCommands";

afterEach(async () => {
  await resetContainer();
});

describe("Runtime Goal data provider", () => {
  it("does not hand a retained command promise to a replacement adapter generation", async () => {
    const input = { sessionId: "ses_1", objective: "Keep one generation" };
    const transportFailure = new RpcTransportError("retired goal response was lost");
    const retiredRetry = vi.fn(
      () =>
        Object.assign(Promise.resolve(runtimeGoal("ses_retired")), {
          idempotencyKey: "retired-goal-start",
          retry: vi.fn(),
        }) as MutationPromise<Goal>,
    );
    const retiredStart = vi.fn(
      () =>
        Object.assign(Promise.reject(transportFailure), {
          idempotencyKey: "retired-goal-start",
          retry: retiredRetry,
        }) as ReturnType<LyraClient["goals"]["start"]>,
    );
    setContainer({
      client: () => ({ goals: { start: retiredStart } }) as unknown as LyraClient,
    });
    const contributor = { contribute: vi.fn() } as unknown as Contributor;
    let adapter = installGoalRuntimeAdapter(contributor);

    await expect(startGoal(input)).rejects.toBe(transportFailure);
    adapter.dispose();

    const successorStart = vi.fn(
      () =>
        Object.assign(Promise.resolve(runtimeGoal("ses_1")), {
          idempotencyKey: "successor-goal-start",
          retry: vi.fn(),
        }) as ReturnType<LyraClient["goals"]["start"]>,
    );
    setContainer({
      client: () => ({ goals: { start: successorStart } }) as unknown as LyraClient,
    });
    adapter = installGoalRuntimeAdapter(contributor);
    try {
      await expect(startGoal(input)).resolves.toBeUndefined();
      expect(successorStart).toHaveBeenCalledOnce();
      expect(retiredRetry).not.toHaveBeenCalled();
    } finally {
      adapter.dispose();
    }
  });

  it("propagates the query generation signal to goals.get", async () => {
    const get = vi.fn().mockResolvedValue(null);
    setContainer({
      client: () => ({ goals: { get } }) as unknown as LyraClient,
    });
    let adapter!: GoalRuntimeAdapterInstallation;
    await contributeForTest((ctx) => {
      adapter = installGoalRuntimeAdapter(ctx);
    });
    try {
      const fetcher = lookupDataProvider<GoalState, GoalQuery>(GOAL_KEY);
      expect(fetcher).toBeDefined();
      const controller = new AbortController();

      await expect(
        fetcher!({ sessionId: "ses_goal_generation" }, controller.signal),
      ).resolves.toEqual({ available: true, goal: null });

      expect(get).toHaveBeenCalledWith("ses_goal_generation", controller.signal);
    } finally {
      adapter.dispose();
    }
  });
});

function runtimeGoal(sessionId: string): Goal {
  return {
    sessionId,
    objective: "Keep one generation",
    status: "active",
    budget: {},
    used: { runs: 0, costUsd: 0, steps: 0 },
    createdAt: "2026-08-17T00:00:00Z",
    updatedAt: "2026-08-17T00:00:00Z",
  };
}
