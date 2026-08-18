import { afterEach, describe, expect, it, vi } from "vitest";
import { AGENT_SESSION_PORTS } from "@/plugins/builtin/agent/public/ports";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import recipesSlash from "./index";

afterEach(async () => {
  await resetKernelForTest();
});

describe("Recipe slash bootstrap", () => {
  it("reads setup-time session identity from its declared service", async () => {
    const activeSessionId = vi.fn(() => "");
    const subscribeActiveSessionId = vi.fn(() => () => undefined);
    const sessions = definePlugin({
      name: "test.recipe-session-ports",
      provides: { sessions: AGENT_SESSION_PORTS },
      setup() {
        return {
          sessions: {
            activeSessionId,
            lifecycleSnapshot: () => ({ activeSessionId: "", openSessionIds: [] }),
            subscribeActiveSessionId,
            subscribeLifecycle: () => () => undefined,
          },
        };
      },
    });

    await loadPluginsForTest(recipesSlash, sessions);

    expect(activeSessionId).toHaveBeenCalled();
    expect(subscribeActiveSessionId).toHaveBeenCalledOnce();
  });
});
