import { afterEach, describe, expect, it, vi } from "vitest";
import { RpcError, RpcTransportError } from "./errors";
import { createMutationPromise } from "./mutation";

afterEach(() => vi.useRealTimers());

describe("mutation settlement", () => {
  it.each([
    new RpcError({
      code: -32002,
      message: "session busy",
      data: { type: "session_busy" },
    }),
    new RpcTransportError("unauthorized", 401),
  ])("does not replay a definitive failure", async (failure) => {
    const execute = vi.fn().mockRejectedValue(failure);

    await expect(createMutationPromise(execute)).rejects.toBe(failure);
    expect(execute).toHaveBeenCalledOnce();
  });

  it("bounds automatic transport recovery to one replay", async () => {
    const failure = new RpcTransportError("connection unavailable");
    const execute = vi.fn().mockRejectedValue(failure);

    await expect(createMutationPromise(execute)).rejects.toBe(failure);
    expect(execute).toHaveBeenCalledTimes(2);
  });

  it("bounds in-progress recovery to one server-directed wait", async () => {
    vi.useFakeTimers();
    const inProgress = new RpcError({
      code: -32021,
      message: "still executing",
      data: { type: "idempotency_in_progress", retryAfterSeconds: 2 },
    });
    const execute = vi.fn().mockRejectedValue(inProgress);
    const mutation = createMutationPromise(execute);
    const settlement = mutation.then(
      () => undefined,
      (error: unknown) => error,
    );

    await vi.advanceTimersByTimeAsync(1_999);
    expect(execute).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(1);
    expect(await settlement).toBe(inProgress);
    expect(execute).toHaveBeenCalledTimes(2);
  });

  it("does not replay a transport failure after the caller cancels", async () => {
    const controller = new AbortController();
    const execute = vi.fn(async () => {
      controller.abort();
      throw new RpcTransportError("fetch failed");
    });

    const mutation = createMutationPromise(execute, "logical-command", {
      signal: controller.signal,
    });

    await expect(mutation).rejects.toBeInstanceOf(RpcTransportError);
    expect(execute).toHaveBeenCalledOnce();
  });

  it("can retry the same logical command with a fresh attempt signal", async () => {
    const expired = new AbortController();
    expired.abort();
    const fresh = new AbortController();
    const execute = vi.fn(async (_key: string, options?: { signal?: AbortSignal }) => {
      if (options?.signal?.aborted) throw new RpcTransportError("attempt expired");
      return "settled";
    });

    const mutation = createMutationPromise(execute, "logical-command", {
      signal: expired.signal,
    });
    await expect(mutation).rejects.toBeInstanceOf(RpcTransportError);
    await expect(mutation.retry({ signal: fresh.signal })).resolves.toBe("settled");

    expect(execute.mock.calls.map(([key]) => key)).toEqual(["logical-command", "logical-command"]);
    expect(execute.mock.calls[1]?.[1]?.signal).toBe(fresh.signal);
  });
});
