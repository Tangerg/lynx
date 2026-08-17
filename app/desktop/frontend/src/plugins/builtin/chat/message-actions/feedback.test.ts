import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";
import { submitMessageFeedback } from "./application/feedback";
import { messageFeedback } from "./feedback";

afterEach(async () => {
  await resetKernelForTest();
  resetContainer();
});

describe("message feedback Runtime generation wiring", () => {
  it("retires an admitted command when the Runtime process generation changes", async () => {
    const pending = deferred<void>();
    const create = vi.fn(() => pending.promise);
    setContainer({ client: () => ({ feedback: { create } }) as unknown as LyraClient });
    let generation = "runtime_1";
    const subscribers = new Set<() => void>();
    const runtime = definePlugin({
      name: "test.runtime-generation",
      provides: { stream: RUNTIME_STREAM_PORTS },
      setup() {
        return {
          stream: {
            runtimeGeneration: () => generation,
            subscribeConnection(onChange: () => void) {
              subscribers.add(onChange);
              return () => subscribers.delete(onChange);
            },
            verifyServiceConnection: vi.fn(),
          },
        };
      },
    });
    await loadPluginsForTest(runtime, messageFeedback);

    const command = rejected(
      submitMessageFeedback(
        {
          sessionId: "ses_feedback",
          messageId: "item_feedback",
          runId: "run_feedback",
        },
        "positive",
      ),
    );
    await vi.waitFor(() => expect(create).toHaveBeenCalledOnce());

    generation = "runtime_2";
    for (const subscriber of subscribers) subscriber();
    await expect(command).resolves.toMatchObject({
      message: "message_feedback_generation_retired",
    });

    pending.resolve();
  });
});

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
