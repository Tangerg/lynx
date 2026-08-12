import { afterEach, describe, expect, it, vi } from "vitest";
import type { MutationPromise } from "./mutation";
import { settleUnaryMutation } from "./mutationSettlement";

afterEach(() => vi.useRealTimers());

function resolvedMutation<T>(value: T): MutationPromise<T> {
  return Object.assign(Promise.resolve(value), {
    idempotencyKey: "same-key",
    retry: vi.fn(),
  });
}

describe("settleUnaryMutation", () => {
  it("replays a timed-out attempt with the same logical mutation", async () => {
    vi.useFakeTimers();
    const retry = vi.fn((_options?: { signal?: AbortSignal }) => resolvedMutation("committed"));
    const open = vi.fn((signal: AbortSignal) =>
      Object.assign(
        new Promise<string>((_resolve, reject) => {
          signal.addEventListener("abort", () => reject(signal.reason), { once: true });
        }),
        { idempotencyKey: "same-key", retry },
      ),
    );

    const result = settleUnaryMutation(open, 10);
    await vi.advanceTimersByTimeAsync(10);

    await expect(result).resolves.toBe("committed");
    expect(open).toHaveBeenCalledOnce();
    expect(retry).toHaveBeenCalledOnce();
    expect(retry.mock.calls[0]?.[0]?.signal).toBeInstanceOf(AbortSignal);
  });

  it("settles after the finite retry budget even when the transport ignores abort", async () => {
    vi.useFakeTimers();
    const never = new Promise<string>(() => {});
    let mutation!: MutationPromise<string>;
    const retry = vi.fn(() => mutation);
    mutation = Object.assign(never, { idempotencyKey: "same-key", retry });
    const open = vi.fn(() => mutation);

    const result = settleUnaryMutation(open, 10);
    const settlement = result.then(
      () => null,
      (error: unknown) => error,
    );
    await vi.advanceTimersByTimeAsync(20);

    await expect(settlement).resolves.toMatchObject({ name: "TimeoutError" });
    expect(open).toHaveBeenCalledOnce();
    expect(retry).toHaveBeenCalledOnce();
  });
});
