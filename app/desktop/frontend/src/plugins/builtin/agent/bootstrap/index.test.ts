import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";
import { forgetRules } from "../application/approvalPolicy";
import { APPROVAL_RULES_KEY } from "../application/approvalPolicyQueries";
import agentBootstrap from "./index";

afterEach(async () => {
  await resetKernelForTest();
  resetContainer();
  queryClient.removeQueries({ queryKey: [APPROVAL_RULES_KEY] });
});

describe("Agent bootstrap Runtime generation wiring", () => {
  it("retires an admitted approval command when the Runtime process generation changes", async () => {
    const retired = deferred();
    const forgetRule = vi.fn(() => retired.promise);
    setContainer({
      client: () => ({ approval: { forgetRule } }) as unknown as LyraClient,
    });
    let generation = "runtime_1";
    const subscribers = new Set<() => void>();
    const runtime = definePlugin({
      name: "test.runtime-generation",
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
    await loadPluginsForTest(runtime, agentBootstrap);

    const command = rejected(forgetRules(["rule-1"]));
    await vi.waitFor(() => expect(forgetRule).toHaveBeenCalledOnce());

    generation = "runtime_2";
    for (const subscriber of subscribers) subscriber();
    await expect(command).resolves.toMatchObject({ message: "agent_command_owner_retired" });

    retired.resolve();
  });
});

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((settle) => {
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
