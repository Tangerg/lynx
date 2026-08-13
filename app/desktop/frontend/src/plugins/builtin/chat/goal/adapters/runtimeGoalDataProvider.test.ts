import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/plugins/builtin/runtime/public/capabilities", () => ({
  runtimeCapability: () => true,
}));

import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import type { Disposable } from "@/plugins/sdk";
import { createHost } from "@/plugins/sdk/host";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { usePluginStore } from "@/plugins/sdk/registry";
import { GOAL_KEY, type GoalQuery, type GoalState } from "../application/goalQueries";
import { installGoalRuntimeAdapter } from "./runtimeGoalCommandsGateway";

let disposables: Disposable[] = [];

beforeEach(() => {
  usePluginStore.getState().resetForTest();
  disposables = [];
});

afterEach(async () => {
  for (const disposable of disposables.reverse()) disposable.dispose();
  await resetContainer();
  usePluginStore.getState().resetForTest();
});

describe("Runtime Goal data provider", () => {
  it("propagates the query generation signal to goals.get", async () => {
    const get = vi.fn().mockResolvedValue(null);
    setContainer({
      client: () => ({ goals: { get } }) as unknown as LyraClient,
    });
    const restoreGateway = installGoalRuntimeAdapter(
      createHost("goal-data-provider-test", disposables),
    );
    try {
      const fetcher = lookupDataProvider<GoalState, GoalQuery>(GOAL_KEY);
      expect(fetcher).toBeDefined();
      const controller = new AbortController();

      await expect(
        fetcher!({ sessionId: "ses_goal_generation" }, controller.signal),
      ).resolves.toEqual({ available: true, goal: null });

      expect(get).toHaveBeenCalledWith("ses_goal_generation", controller.signal);
    } finally {
      restoreGateway();
    }
  });
});
