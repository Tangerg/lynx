import { describe, expect, it, vi } from "vitest";
import type { WorkspaceEventLoop } from "./workspaceEventLoop";
import {
  startWorkspaceEventSubscription,
  type WorkspaceEventSubscriptionPorts,
} from "./workspaceEventSubscription";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));

function subscriptionPorts(
  patch: Partial<WorkspaceEventSubscriptionPorts> = {},
): WorkspaceEventSubscriptionPorts {
  const loop: WorkspaceEventLoop = {
    start: vi.fn(),
    retarget: vi.fn(),
  };
  return {
    canSubscribe: vi.fn(() => true),
    subscribeCapabilities: vi.fn(() => vi.fn()),
    resolveWorkspaceCwd: vi.fn().mockResolvedValue("/repo"),
    subscribeWorkspaceCwdInputs: vi.fn(() => vi.fn()),
    loop,
    ...patch,
  };
}

describe("startWorkspaceEventSubscription", () => {
  it("starts immediately when the runtime advertises workspace.subscribe", () => {
    const ports = subscriptionPorts();

    startWorkspaceEventSubscription(ports);

    expect(ports.loop.start).toHaveBeenCalledOnce();
    expect(ports.loop.start).toHaveBeenCalledWith(expect.any(AbortSignal));
  });

  it("starts once when the capability is advertised later", () => {
    let onCapabilitiesChange: (() => void) | undefined;
    let advertised = false;
    const ports = subscriptionPorts({
      canSubscribe: () => advertised,
      subscribeCapabilities: (listener) => {
        onCapabilitiesChange = listener;
        return vi.fn();
      },
    });

    startWorkspaceEventSubscription(ports);
    expect(ports.loop.start).not.toHaveBeenCalled();

    advertised = true;
    onCapabilitiesChange?.();
    onCapabilitiesChange?.();

    expect(ports.loop.start).toHaveBeenCalledOnce();
  });

  it("retargets to the latest resolved cwd and ignores stale resolutions", async () => {
    const first = deferred<string | undefined>();
    const second = deferred<string | undefined>();
    let onCwdChange: (() => void) | undefined;
    const resolveWorkspaceCwd = vi
      .fn<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>()
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);
    const ports = subscriptionPorts({
      resolveWorkspaceCwd,
      subscribeWorkspaceCwdInputs: (listener) => {
        onCwdChange = listener;
        return vi.fn();
      },
    });

    startWorkspaceEventSubscription(ports);
    onCwdChange?.();
    first.resolve("/old");
    await tick();
    second.resolve("/new");
    await tick();

    expect(ports.loop.retarget).toHaveBeenCalledTimes(1);
    expect(ports.loop.retarget).toHaveBeenCalledWith("/new");
  });

  it("unsubscribes, aborts the loop signal, and suppresses late retargets on dispose", async () => {
    const unsubscribeCapabilities = vi.fn();
    const unsubscribeCwdInputs = vi.fn();
    const cwd = deferred<string | undefined>();
    let loopSignal: AbortSignal | undefined;
    const loop: WorkspaceEventLoop = {
      start: vi.fn((signal) => {
        loopSignal = signal;
      }),
      retarget: vi.fn(),
    };
    const ports = subscriptionPorts({
      loop,
      resolveWorkspaceCwd: vi.fn().mockReturnValue(cwd.promise),
      subscribeCapabilities: vi.fn(() => unsubscribeCapabilities),
      subscribeWorkspaceCwdInputs: vi.fn(() => unsubscribeCwdInputs),
    });

    const dispose = startWorkspaceEventSubscription(ports);
    dispose();
    cwd.resolve("/repo");
    await tick();

    expect(unsubscribeCapabilities).toHaveBeenCalledOnce();
    expect(unsubscribeCwdInputs).toHaveBeenCalledOnce();
    expect(loopSignal?.aborted).toBe(true);
    expect(loop.retarget).not.toHaveBeenCalled();
  });
});
