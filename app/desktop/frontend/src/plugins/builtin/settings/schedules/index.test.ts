import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { resetContainer, setContainer } from "@/main/container";
import type { ScopeAppClient } from "@/rpc";
import { definePlugin } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";
import { runScheduleNow } from "./application/scheduleCommands";
import { SCHEDULES_KEY } from "./application/scheduleQueries";
import schedulesPlugin from "./index";

const { selectAgentSession } = vi.hoisted(() => ({ selectAgentSession: vi.fn() }));

vi.mock("@/plugins/builtin/agent/public/session", () => ({ selectAgentSession }));

afterEach(async () => {
  await resetKernelForTest();
  resetContainer();
  queryClient.removeQueries({ queryKey: [SCHEDULES_KEY] });
  selectAgentSession.mockReset();
});

describe("schedules plugin Runtime generation wiring", () => {
  it("retires run-now navigation when the Runtime process generation changes", async () => {
    const retired = deferred<{ sessionId: string; runId: string }>();
    const runNow = vi.fn(() => retired.promise);
    setContainer({
      client: () => ({ schedules: { runNow } }) as unknown as ScopeAppClient,
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
    await loadPluginsForTest(runtime, schedulesPlugin);

    const command = rejected(runScheduleNow("schedule-1"));
    await vi.waitFor(() => expect(runNow).toHaveBeenCalledOnce());

    generation = "runtime_2";
    for (const subscriber of subscribers) subscriber();
    await expect(command).resolves.toMatchObject({
      message: "schedule_mutation_generation_retired",
    });
    expect(selectAgentSession).not.toHaveBeenCalled();

    retired.resolve({ sessionId: "ses_retired", runId: "run_retired" });
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
