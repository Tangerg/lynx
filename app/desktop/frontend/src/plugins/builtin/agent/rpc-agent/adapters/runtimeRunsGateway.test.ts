import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import {
  asItemId,
  asRunId,
  asSegmentId,
  RpcTransportError,
  type LyraClient,
  type MutationPromise,
  type RunEvent,
} from "@/rpc";
import { createMutationPromise } from "@/rpc/mutation";
import { RUN_OPENING_ATTEMPT_TIMEOUT_MS } from "./runOpeningSettlement";
import { runtimeRunsGateway } from "./runtimeRunsGateway";

afterEach(() => {
  resetContainer();
  vi.useRealTimers();
});

async function* noEvents(): AsyncIterable<RunEvent> {}

describe("runtimeRunsGateway", () => {
  it("does not hand a retained opening to a replacement gateway generation", async () => {
    const params = { sessionId: "ses_1", input: [{ type: "text" as const, text: "ship it" }] };
    const transportFailure = new RpcTransportError("retired opening response was lost");
    const retiredRetry = vi.fn(
      () =>
        Object.assign(
          Promise.resolve({
            result: {
              runId: asRunId("run_retired"),
              segmentId: asSegmentId("seg_retired"),
              userItemId: asItemId("item_retired"),
            },
            events: noEvents(),
          }),
          { idempotencyKey: "retired-run-start", retry: vi.fn() },
        ) as MutationPromise<{
          result: {
            runId: ReturnType<typeof asRunId>;
            segmentId: ReturnType<typeof asSegmentId>;
            userItemId: ReturnType<typeof asItemId>;
          };
          events: AsyncIterable<RunEvent>;
        }>,
    );
    const retiredStart = vi.fn(
      () =>
        Object.assign(Promise.reject(transportFailure), {
          idempotencyKey: "retired-run-start",
          retry: retiredRetry,
        }) as ReturnType<LyraClient["runs"]["start"]>,
    );
    setContainer({
      client: () => ({ runs: { start: retiredStart } }) as unknown as LyraClient,
    });
    const retiredGateway = runtimeRunsGateway();

    await expect(retiredGateway.start(params)).rejects.toBe(transportFailure);

    const successorStart = vi.fn(
      () =>
        Object.assign(
          Promise.resolve({
            result: {
              runId: asRunId("run_successor"),
              segmentId: asSegmentId("seg_successor"),
              userItemId: asItemId("item_successor"),
            },
            events: noEvents(),
          }),
          { idempotencyKey: "successor-run-start", retry: vi.fn() },
        ) as ReturnType<LyraClient["runs"]["start"]>,
    );
    setContainer({
      client: () => ({ runs: { start: successorStart } }) as unknown as LyraClient,
    });
    const successorGateway = runtimeRunsGateway();

    await expect(successorGateway.start(params)).resolves.toMatchObject({
      result: {
        runId: "run_successor",
        segmentId: "seg_successor",
        userItemId: "item_successor",
      },
    });
    expect(successorStart).toHaveBeenCalledOnce();
    expect(retiredRetry).not.toHaveBeenCalled();
  });

  it("keeps one runs.start identity across a product-level retry", async () => {
    vi.useFakeTimers();
    const keys: string[] = [];
    let executions = 0;
    const start = vi.fn((_params, signal?: AbortSignal) =>
      createMutationPromise(
        async (key, attempt) => {
          keys.push(key);
          executions += 1;
          if (executions === 3) {
            return {
              result: {
                runId: asRunId("run_1"),
                segmentId: asSegmentId("seg_1"),
                userItemId: asItemId("item_1"),
              },
              events: noEvents(),
            };
          }
          await new Promise<never>((_resolve, reject) => {
            attempt.signal?.addEventListener(
              "abort",
              () => reject(new RpcTransportError("opening timed out")),
              { once: true },
            );
          });
          throw new Error("unreachable");
        },
        "logical-run-start",
        { signal },
      ),
    );
    setContainer({
      client: () => ({ runs: { start } }) as unknown as LyraClient,
    });
    const gateway = runtimeRunsGateway();
    const params = { sessionId: "ses_1", input: [{ type: "text" as const, text: "ship it" }] };

    const first = gateway.start(params);
    const firstFailure = first.catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(RUN_OPENING_ATTEMPT_TIMEOUT_MS * 2);
    await expect(firstFailure).resolves.toMatchObject({ name: "TimeoutError" });

    await expect(gateway.start(params)).resolves.toMatchObject({
      result: { runId: "run_1", segmentId: "seg_1", userItemId: "item_1" },
    });
    expect(start).toHaveBeenCalledOnce();
    expect(keys).toEqual(["logical-run-start", "logical-run-start", "logical-run-start"]);
  });
});
