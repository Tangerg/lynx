import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";
import { setHookTrust } from "./application/hookTrust";
import hooksPlugin from "./index";

afterEach(async () => {
  await resetKernelForTest();
  resetContainer();
});

describe("hooks plugin Runtime generation wiring", () => {
  it("retires an admitted trust command when the Runtime process generation changes", async () => {
    const retired = deferred();
    const setTrust = vi.fn(() => retired.promise);
    setContainer({
      client: () => ({ hooks: { setTrust } }) as unknown as LyraClient,
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
    await loadPluginsForTest(runtime, hooksPlugin);

    const command = rejected(setHookTrust("/repo", true));
    await vi.waitFor(() => expect(setTrust).toHaveBeenCalledOnce());

    generation = "runtime_2";
    for (const subscriber of subscribers) subscriber();
    await expect(command).resolves.toMatchObject({
      message: "hook_trust_mutation_generation_retired",
    });

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
