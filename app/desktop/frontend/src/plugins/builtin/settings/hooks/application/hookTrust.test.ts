import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import { HookTrustMutationOwner, setHookTrust } from "./hookTrust";

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((onResolve) => {
    resolve = onResolve;
  });
  return { promise, resolve };
}

let owner: HookTrustMutationOwner | undefined;

afterEach(() => {
  owner?.dispose();
  owner = undefined;
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
    owner = HookTrustMutationOwner.install({ setProjectTrust });

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
    owner = HookTrustMutationOwner.install({
      setProjectTrust: vi.fn().mockRejectedValue(failure),
    });

    await expect(setHookTrust("/repo", true)).rejects.toBe(failure);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["hooks"] });
  });

  it("does not globally block trust changes for unrelated projects", async () => {
    const firstWrite = deferred();
    const setProjectTrust = vi.fn((projectRoot: string) =>
      projectRoot === "/slow" ? firstWrite.promise : Promise.resolve(),
    );
    owner = HookTrustMutationOwner.install({ setProjectTrust });
    vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();

    const slow = setHookTrust("/slow", true);
    await vi.waitFor(() => expect(setProjectTrust).toHaveBeenCalledWith("/slow", true));
    await expect(setHookTrust("/ready", true)).resolves.toBeUndefined();
    expect(setProjectTrust).toHaveBeenCalledWith("/ready", true);

    firstWrite.resolve();
    await expect(slow).resolves.toBeUndefined();
  });

  it("keeps an accepted trust command successful when cache repair fails", async () => {
    owner = HookTrustMutationOwner.install({
      setProjectTrust: vi.fn().mockResolvedValue(undefined),
    });
    vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("cache unavailable"));

    await expect(setHookTrust("/repo", true)).resolves.toBeUndefined();
  });
});
