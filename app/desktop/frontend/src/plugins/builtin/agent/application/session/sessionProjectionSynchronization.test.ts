import { describe, expect, it, vi } from "vitest";
import { createSessionProjectionSynchronization } from "./sessionProjectionSynchronization";

function deferred() {
  let resolve!: (committed: boolean) => void;
  const promise = new Promise<boolean>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("session projection synchronization", () => {
  it("coalesces snapshot requests behind the active live-stream owner", async () => {
    let active = true;
    const synchronize = vi.fn().mockResolvedValue(true);
    const coordinator = createSessionProjectionSynchronization({
      isLiveStreamActive: () => active,
      synchronize,
    });

    const firstRequest = coordinator.request();
    const secondRequest = coordinator.request();
    expect(synchronize).not.toHaveBeenCalled();

    active = false;
    coordinator.liveStreamSettled();
    await vi.waitFor(() => expect(synchronize).toHaveBeenCalledTimes(1));
    await expect(firstRequest).resolves.toBe(true);
    await expect(secondRequest).resolves.toBe(true);
  });

  it("serializes a change arriving during snapshot reconciliation", async () => {
    const first = deferred();
    const synchronize = vi
      .fn<() => Promise<boolean>>()
      .mockImplementationOnce(() => first.promise)
      .mockResolvedValue(true);
    const coordinator = createSessionProjectionSynchronization({
      isLiveStreamActive: () => false,
      synchronize,
    });

    void coordinator.request();
    void coordinator.request();
    expect(synchronize).toHaveBeenCalledTimes(1);

    first.resolve(true);
    await vi.waitFor(() => expect(synchronize).toHaveBeenCalledTimes(2));
  });

  it("retains a newer snapshot request when a live stream takes ownership mid-refresh", async () => {
    let active = false;
    const first = deferred();
    const synchronize = vi
      .fn<() => Promise<boolean>>()
      .mockImplementationOnce(() => first.promise)
      .mockResolvedValue(true);
    const coordinator = createSessionProjectionSynchronization({
      isLiveStreamActive: () => active,
      synchronize,
    });

    void coordinator.request();
    active = true;
    void coordinator.request();
    first.resolve(true);
    await Promise.resolve();
    await Promise.resolve();
    expect(synchronize).toHaveBeenCalledTimes(1);

    active = false;
    coordinator.liveStreamSettled();
    await vi.waitFor(() => expect(synchronize).toHaveBeenCalledTimes(2));
  });

  it("drops retained work after disposal", async () => {
    let active = true;
    const synchronize = vi.fn().mockResolvedValue(true);
    const coordinator = createSessionProjectionSynchronization({
      isLiveStreamActive: () => active,
      synchronize,
    });

    const retained = coordinator.request();
    coordinator.dispose();
    active = false;
    coordinator.liveStreamSettled();

    expect(synchronize).not.toHaveBeenCalled();
    await expect(retained).resolves.toBe(false);
  });

  it("does not drain retained work after an in-flight refresh is disposed", async () => {
    const first = deferred();
    const synchronize = vi.fn(() => first.promise);
    const coordinator = createSessionProjectionSynchronization({
      isLiveStreamActive: () => false,
      synchronize,
    });

    const inFlight = coordinator.request();
    const retained = coordinator.request();
    coordinator.dispose();
    await expect(inFlight).resolves.toBe(false);
    await expect(retained).resolves.toBe(false);
    first.resolve(true);
    await Promise.resolve();
    await Promise.resolve();

    expect(synchronize).toHaveBeenCalledTimes(1);
  });
});
