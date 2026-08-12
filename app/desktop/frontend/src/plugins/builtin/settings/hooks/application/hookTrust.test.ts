import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { configureHookTrustGateway } from "./ports/hookTrustGateway";
import { setHookTrust } from "./hookTrust";

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((onResolve) => {
    resolve = onResolve;
  });
  return { promise, resolve };
}

let restoreGateway: (() => void) | undefined;

afterEach(() => {
  restoreGateway?.();
  restoreGateway = undefined;
  vi.restoreAllMocks();
});

describe("hook trust mutation", () => {
  it("serializes trust intent per project through authoritative revalidation", async () => {
    const firstWrite = deferred();
    const firstRefresh = deferred();
    const setProjectTrust = vi
      .fn()
      .mockImplementationOnce(() => firstWrite.promise)
      .mockResolvedValueOnce(undefined);
    const invalidate = vi
      .spyOn(queryClient, "invalidateQueries")
      .mockImplementationOnce(() => firstRefresh.promise)
      .mockResolvedValueOnce(undefined);
    restoreGateway = configureHookTrustGateway({ setProjectTrust });

    const trust = setHookTrust("/repo", true);
    const untrust = setHookTrust("/repo", false);
    await vi.waitFor(() => expect(setProjectTrust).toHaveBeenCalledTimes(1));

    firstWrite.resolve();
    await vi.waitFor(() => expect(invalidate).toHaveBeenCalledTimes(1));
    expect(setProjectTrust).toHaveBeenCalledTimes(1);

    firstRefresh.resolve();
    await expect(trust).resolves.toBeUndefined();
    await expect(untrust).resolves.toBeUndefined();
    expect(setProjectTrust.mock.calls).toEqual([
      ["/repo", true],
      ["/repo", false],
    ]);
  });

  it("preserves a failed command while repairing the cached read", async () => {
    const failure = new Error("trust response lost");
    const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
    restoreGateway = configureHookTrustGateway({
      setProjectTrust: vi.fn().mockRejectedValue(failure),
    });

    await expect(setHookTrust("/repo", true)).rejects.toBe(failure);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["hooks"] });
  });
});
