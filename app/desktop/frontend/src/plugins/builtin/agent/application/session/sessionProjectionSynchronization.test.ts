import { describe, expect, it, vi } from "vitest";
import { createSessionProjectionSynchronization } from "./sessionProjectionSynchronization";

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("session projection synchronization", () => {
  it("coalesces snapshot requests behind the active live-stream owner", async () => {
    let active = true;
    const synchronize = vi.fn().mockResolvedValue(undefined);
    const coordinator = createSessionProjectionSynchronization({
      isLiveStreamActive: () => active,
      synchronize,
    });

    coordinator.request();
    coordinator.request();
    expect(synchronize).not.toHaveBeenCalled();

    active = false;
    coordinator.liveStreamSettled();
    await vi.waitFor(() => expect(synchronize).toHaveBeenCalledTimes(1));
  });

  it("serializes a change arriving during snapshot reconciliation", async () => {
    const first = deferred();
    const synchronize = vi
      .fn<() => Promise<void>>()
      .mockImplementationOnce(() => first.promise)
      .mockResolvedValue(undefined);
    const coordinator = createSessionProjectionSynchronization({
      isLiveStreamActive: () => false,
      synchronize,
    });

    coordinator.request();
    coordinator.request();
    expect(synchronize).toHaveBeenCalledTimes(1);

    first.resolve();
    await vi.waitFor(() => expect(synchronize).toHaveBeenCalledTimes(2));
  });

  it("retains a newer snapshot request when a live stream takes ownership mid-refresh", async () => {
    let active = false;
    const first = deferred();
    const synchronize = vi
      .fn<() => Promise<void>>()
      .mockImplementationOnce(() => first.promise)
      .mockResolvedValue(undefined);
    const coordinator = createSessionProjectionSynchronization({
      isLiveStreamActive: () => active,
      synchronize,
    });

    coordinator.request();
    active = true;
    coordinator.request();
    first.resolve();
    await Promise.resolve();
    await Promise.resolve();
    expect(synchronize).toHaveBeenCalledTimes(1);

    active = false;
    coordinator.liveStreamSettled();
    await vi.waitFor(() => expect(synchronize).toHaveBeenCalledTimes(2));
  });

  it("drops retained work after disposal", () => {
    let active = true;
    const synchronize = vi.fn().mockResolvedValue(undefined);
    const coordinator = createSessionProjectionSynchronization({
      isLiveStreamActive: () => active,
      synchronize,
    });

    coordinator.request();
    coordinator.dispose();
    active = false;
    coordinator.liveStreamSettled();

    expect(synchronize).not.toHaveBeenCalled();
  });

  it("does not drain retained work after an in-flight refresh is disposed", async () => {
    const first = deferred();
    const synchronize = vi.fn(() => first.promise);
    const coordinator = createSessionProjectionSynchronization({
      isLiveStreamActive: () => false,
      synchronize,
    });

    coordinator.request();
    coordinator.request();
    coordinator.dispose();
    first.resolve();
    await Promise.resolve();
    await Promise.resolve();

    expect(synchronize).toHaveBeenCalledTimes(1);
  });
});
