import { afterEach, describe, expect, it, vi } from "vitest";
import { RpcTransportError, type MutationAttemptOptions, type MutationPromise } from "@/rpc";
import { settleRunOpening } from "./runOpeningSettlement";

afterEach(() => vi.useRealTimers());

describe("run opening settlement", () => {
  it("retries a timed-out opening with the same identity and a fresh signal", async () => {
    vi.useFakeTimers();
    const signals: AbortSignal[] = [];
    const keys: string[] = [];
    let execution = 0;
    const opening = settleRunOpening(
      (signal) =>
        replayableMutation(async (key, attempt) => {
          keys.push(key);
          signals.push(attempt.signal!);
          execution += 1;
          if (execution === 1) return rejectWhenAborted(attempt.signal!);
          return "accepted";
        }, signal),
      undefined,
      1_000,
    );

    await vi.advanceTimersByTimeAsync(1_000);
    await expect(opening).resolves.toBe("accepted");
    expect(keys).toEqual(["run-opening", "run-opening"]);
    expect(signals).toHaveLength(2);
    expect(signals[0]?.aborted).toBe(true);
    expect(signals[1]).not.toBe(signals[0]);
    expect(signals[1]?.aborted).toBe(false);
  });

  it("releases the winning deadline without releasing parent stream ownership", async () => {
    vi.useFakeTimers();
    const parent = new AbortController();
    let winningSignal: AbortSignal | undefined;
    const opening = settleRunOpening(
      (signal) =>
        replayableMutation(async (_key, attempt) => {
          winningSignal = attempt.signal;
          return "accepted";
        }, signal),
      parent.signal,
      1_000,
    );

    await expect(opening).resolves.toBe("accepted");
    await vi.advanceTimersByTimeAsync(10_000);
    expect(winningSignal?.aborted).toBe(false);

    parent.abort();
    expect(winningSignal?.aborted).toBe(true);
  });

  it("does not retry when the session owner cancels the opening", async () => {
    const parent = new AbortController();
    const execute = vi.fn(async (_key: string, attempt: { signal?: AbortSignal }) =>
      rejectWhenAborted(attempt.signal!),
    );
    const opening = settleRunOpening(
      (signal) => replayableMutation(execute, signal),
      parent.signal,
      1_000,
    );

    parent.abort();
    await expect(opening).rejects.toBeInstanceOf(RpcTransportError);
    expect(execute).toHaveBeenCalledOnce();
  });

  it("bounds deadline recovery to two delivery attempts", async () => {
    vi.useFakeTimers();
    const execute = vi.fn(async (_key: string, attempt: { signal?: AbortSignal }) =>
      rejectWhenAborted(attempt.signal!),
    );
    const opening = settleRunOpening(
      (signal) => replayableMutation(execute, signal),
      undefined,
      1_000,
    );
    const settlement = opening.then(
      () => undefined,
      (error: unknown) => error,
    );

    await vi.advanceTimersByTimeAsync(2_000);
    await expect(settlement).resolves.toBeInstanceOf(RpcTransportError);
    expect(execute).toHaveBeenCalledTimes(2);
  });
});

function rejectWhenAborted(signal: AbortSignal): Promise<never> {
  if (signal.aborted) return Promise.reject(new RpcTransportError("opening aborted"));
  return new Promise((_, reject) => {
    signal.addEventListener("abort", () => reject(new RpcTransportError("opening aborted")), {
      once: true,
    });
  });
}

function replayableMutation<T>(
  execute: (key: string, attempt: MutationAttemptOptions) => Promise<T>,
  signal: AbortSignal,
): MutationPromise<T> {
  const key = "run-opening";
  const create = (attempt: MutationAttemptOptions): MutationPromise<T> => {
    const promise = Promise.resolve().then(() => execute(key, attempt));
    return Object.defineProperties(promise, {
      idempotencyKey: { enumerable: true, value: key },
      retry: {
        enumerable: true,
        value: (options: MutationAttemptOptions = attempt) => create(options),
      },
    }) as MutationPromise<T>;
  };
  return create({ signal });
}
