import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import {
  asItemId,
  asRunId,
  asSegmentId,
  RpcTransportError,
  type ScopeAppClient,
  type MutationPromise,
  type RunEvent,
} from "@/rpc";
import { definePlugin, pickAgentSource } from "@/plugins/sdk";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";
import rpcAgent from "./index";

vi.mock("@/plugins/builtin/agent/public/session", () => ({
  getActiveSessionId: () => "ses_1",
}));

afterEach(async () => {
  await resetKernelForTest();
  await resetContainer();
  vi.restoreAllMocks();
});

describe("RPC Agent Runtime generation wiring", () => {
  it("never replays a predecessor Run opening through the successor generation", async () => {
    const predecessorFailure = new RpcTransportError("predecessor response was lost");
    const predecessorRetry = vi.fn(() =>
      mutation(
        Promise.resolve({
          result: {
            runId: asRunId("run_predecessor"),
            segmentId: asSegmentId("seg_predecessor"),
            userItemId: asItemId("item_predecessor"),
          },
          events: noEvents(),
        }),
        "predecessor-opening",
      ),
    );
    const predecessorStart = vi.fn(
      () =>
        Object.assign(Promise.reject(predecessorFailure), {
          idempotencyKey: "predecessor-opening",
          retry: predecessorRetry,
        }) as ReturnType<ScopeAppClient["runs"]["start"]>,
    );
    setContainer({
      client: () => ({ runs: { start: predecessorStart } }) as unknown as ScopeAppClient,
    });

    const runtime = new RuntimeGenerationFixture("test.rpc-agent-runtime-generation");
    await loadPluginsForTest(runtime.plugin, rpcAgent);
    const source = pickAgentSource();
    expect(source?.id).toBe("rpc");
    const driver = source!.factory();
    const input = [{ type: "text" as const, text: "same logical input" }];

    await expect(driver.start(input, {})).rejects.toBe(predecessorFailure);

    const successorStart = vi.fn(() =>
      mutation(
        Promise.resolve({
          result: {
            runId: asRunId("run_successor"),
            segmentId: asSegmentId("seg_successor"),
            userItemId: asItemId("item_successor"),
          },
          events: noEvents(),
        }),
        "successor-opening",
      ),
    );
    setContainer({
      client: () => ({ runs: { start: successorStart } }) as unknown as ScopeAppClient,
    });
    runtime.replace("runtime_2");

    await expect(driver.start(input, {})).resolves.toMatchObject({
      result: { runId: "run_successor", segmentId: "seg_successor" },
    });
    expect(successorStart).toHaveBeenCalledOnce();
    expect(predecessorRetry).not.toHaveBeenCalled();
  });

  it("retires an accepted predecessor stream only when the Runtime identity changes", async () => {
    let acceptedSignal: AbortSignal | undefined;
    const start = vi.fn((_params, signal?: AbortSignal) => {
      acceptedSignal = signal;
      return mutation(
        Promise.resolve({
          result: {
            runId: asRunId("run_accepted"),
            segmentId: asSegmentId("seg_accepted"),
            userItemId: asItemId("item_accepted"),
          },
          events: noEvents(),
        }),
        "accepted-opening",
      );
    });
    setContainer({
      client: () => ({ runs: { start } }) as unknown as ScopeAppClient,
    });

    const runtime = new RuntimeGenerationFixture("test.rpc-agent-accepted-stream-generation");
    await loadPluginsForTest(runtime.plugin, rpcAgent);
    await pickAgentSource()!
      .factory()
      .start([{ type: "text", text: "accepted" }], {});
    expect(acceptedSignal?.aborted).toBe(false);

    runtime.notify();
    expect(acceptedSignal?.aborted).toBe(false);

    runtime.replace("runtime_2");
    expect(acceptedSignal?.aborted).toBe(true);
  });
});

class RuntimeGenerationFixture {
  readonly #subscribers = new Set<() => void>();
  #generation = "runtime_1";
  readonly plugin;

  constructor(name: string) {
    this.plugin = definePlugin({
      name,
      provides: { stream: RUNTIME_STREAM_PORTS },
      setup: () => ({
        stream: {
          connectionGeneration: () => this.#generation,
          subscribeConnection: (onChange: () => void) => {
            this.#subscribers.add(onChange);
            return () => this.#subscribers.delete(onChange);
          },
          reportConnectionLoss: vi.fn(),
        },
      }),
    });
  }

  notify(): void {
    for (const subscriber of this.#subscribers) subscriber();
  }

  replace(generation: string): void {
    this.#generation = generation;
    this.notify();
  }
}

function mutation<T>(promise: Promise<T>, idempotencyKey: string): MutationPromise<T> {
  return Object.assign(promise, {
    idempotencyKey,
    retry: vi.fn(),
  });
}

async function* noEvents(): AsyncIterable<RunEvent> {}
