import { afterEach, describe, expect, it, vi } from "vitest";
import { RpcTransportError, type MutationAttemptOptions, type MutationPromise } from "@/rpc";
import { createRunOpeningSettler } from "./runOpeningSettlement";

afterEach(() => vi.useRealTimers());

describe("run opening settlement", () => {
  it("retries a timed-out opening with the same identity and a fresh signal", async () => {
    vi.useFakeTimers();
    const settler = createRunOpeningSettler();
    const signals: AbortSignal[] = [];
    const keys: string[] = [];
    let execution = 0;
    const opening = settler.settle(
      "test:timeout-replay",
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
    settler.dispose();
  });

  it("releases the winning deadline without releasing parent stream ownership", async () => {
    vi.useFakeTimers();
    const settler = createRunOpeningSettler();
    const parent = new AbortController();
    let winningSignal: AbortSignal | undefined;
    const opening = settler.settle(
      "test:parent-lifetime",
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
    settler.dispose();
  });

  it("does not retry when the session owner cancels the opening", async () => {
    const parent = new AbortController();
    const settler = createRunOpeningSettler();
    const execute = vi.fn(async (_key: string, attempt: { signal?: AbortSignal }) =>
      rejectWhenAborted(attempt.signal!),
    );
    const opening = settler.settle(
      "test:parent-cancel",
      (signal) => replayableMutation(execute, signal),
      parent.signal,
      1_000,
    );

    parent.abort();
    await expect(opening).rejects.toMatchObject({ name: "AbortError" });
    expect(execute).toHaveBeenCalledOnce();
    settler.dispose();
  });

  it("bounds deadline recovery to two delivery attempts", async () => {
    vi.useFakeTimers();
    const settler = createRunOpeningSettler();
    const execute = vi.fn(async (_key: string, attempt: { signal?: AbortSignal }) =>
      rejectWhenAborted(attempt.signal!),
    );
    const opening = settler.settle(
      "test:finite-budget",
      (signal) => replayableMutation(execute, signal),
      undefined,
      1_000,
    );
    const settlement = opening.then(
      () => undefined,
      (error: unknown) => error,
    );

    await vi.advanceTimersByTimeAsync(2_000);
    await expect(settlement).resolves.toMatchObject({ name: "TimeoutError" });
    expect(execute).toHaveBeenCalledTimes(2);
    settler.dispose();
  });

  it("settles its finite budget even when the transport ignores cancellation", async () => {
    vi.useFakeTimers();
    const settler = createRunOpeningSettler();
    let settleIgnored!: (value: string) => void;
    const ignored = new Promise<string>((resolve) => {
      settleIgnored = resolve;
    });
    let mutation!: MutationPromise<string>;
    mutation = Object.assign(ignored, {
      idempotencyKey: "run-opening",
      retry: vi.fn(() => mutation),
    });

    const opening = settler.settle("test:ignored-cancellation", () => mutation, undefined, 1_000);
    const settlement = opening.catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(2_000);

    await expect(settlement).resolves.toMatchObject({ name: "TimeoutError" });
    expect(mutation.retry).toHaveBeenCalledOnce();
    settler.dispose();
    settleIgnored("late ignored opening");
    await ignored;
  });

  it("replays a retained opening after bounded settlement returned to the product", async () => {
    vi.useFakeTimers();
    let settleIgnored!: (value: string) => void;
    const ignored = new Promise<string>((resolve) => {
      settleIgnored = resolve;
    });
    const retry = vi.fn(() =>
      retry.mock.calls.length === 2
        ? replayableMutation(async () => "accepted", new AbortController().signal)
        : (Object.assign(ignored, {
            idempotencyKey: "run-opening",
            retry,
          }) as MutationPromise<string>),
    );
    const open = vi.fn(
      () =>
        Object.assign(ignored, {
          idempotencyKey: "run-opening",
          retry,
        }) as MutationPromise<string>,
    );
    const settler = createRunOpeningSettler();

    const first = settler.settle("runs.resume:run_1", open, undefined, 1_000);
    const firstFailure = first.catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(2_000);
    await expect(firstFailure).resolves.toMatchObject({ name: "TimeoutError" });

    await expect(settler.settle("runs.resume:run_1", open, undefined, 1_000)).resolves.toBe(
      "accepted",
    );
    expect(open).toHaveBeenCalledOnce();
    expect(retry).toHaveBeenCalledTimes(2);
    settleIgnored("late ignored opening");
    await ignored;
  });

  it("retains a parent-canceled opening for a later owner without retrying immediately", async () => {
    const firstOwner = new AbortController();
    const retry = vi.fn(() =>
      replayableMutation(async () => "accepted", new AbortController().signal),
    );
    const open = vi.fn(
      (signal: AbortSignal) =>
        Object.assign(rejectWhenAborted(signal), {
          idempotencyKey: "run-opening",
          retry,
        }) as MutationPromise<string>,
    );
    const settler = createRunOpeningSettler();

    const first = settler.settle("runs.start:ses_1", open, firstOwner.signal, 1_000);
    firstOwner.abort();
    await expect(first).rejects.toBeDefined();
    expect(retry).not.toHaveBeenCalled();

    await expect(
      settler.settle("runs.start:ses_1", open, new AbortController().signal, 1_000),
    ).resolves.toBe("accepted");
    expect(open).toHaveBeenCalledTimes(1);
    expect(retry).toHaveBeenCalledOnce();
  });

  it("revokes an accepted event stream when its adapter generation is disposed", async () => {
    let acceptedSignal: AbortSignal | undefined;
    const settler = createRunOpeningSettler();
    await expect(
      settler.settle(
        "runs.start:ses_1",
        (signal) =>
          replayableMutation(async (_key, attempt) => {
            acceptedSignal = attempt.signal;
            return "accepted";
          }, signal),
        undefined,
        60_000,
      ),
    ).resolves.toBe("accepted");
    expect(acceptedSignal?.aborted).toBe(false);

    settler.dispose();

    expect(acceptedSignal?.aborted).toBe(true);
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
