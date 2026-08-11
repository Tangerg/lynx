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
    resolveWorkspaceCwd: vi.fn().mockResolvedValue({ status: "resolved", cwd: "/repo" }),
    subscribeWorkspaceCwdInputs: vi.fn(() => vi.fn()),
    loop,
    ...patch,
  };
}

describe("startWorkspaceEventSubscription", () => {
  it("starts immediately when the runtime advertises runtime.subscribe", () => {
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
    const first =
      deferred<Awaited<ReturnType<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>>>();
    const second =
      deferred<Awaited<ReturnType<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>>>();
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
    first.resolve({ status: "resolved", cwd: "/old" });
    await tick();
    second.resolve({ status: "resolved", cwd: "/new" });
    await tick();

    expect(ports.loop.retarget).toHaveBeenCalledTimes(1);
    expect(ports.loop.retarget).toHaveBeenCalledWith("/new");
  });

  it("retargets to the default workspace but preserves the target on resolution failure", async () => {
    const unavailable =
      deferred<Awaited<ReturnType<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>>>();
    const defaultWorkspace =
      deferred<Awaited<ReturnType<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>>>();
    let onCwdChange: (() => void) | undefined;
    const ports = subscriptionPorts({
      resolveWorkspaceCwd: vi
        .fn<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>()
        .mockReturnValueOnce(unavailable.promise)
        .mockReturnValueOnce(defaultWorkspace.promise),
      subscribeWorkspaceCwdInputs: (listener) => {
        onCwdChange = listener;
        return vi.fn();
      },
    });

    startWorkspaceEventSubscription(ports);
    unavailable.resolve({ status: "unavailable" });
    await tick();
    expect(ports.loop.retarget).not.toHaveBeenCalled();

    onCwdChange?.();
    defaultWorkspace.resolve({ status: "resolved" });
    await tick();
    expect(ports.loop.retarget).toHaveBeenCalledOnce();
    expect(ports.loop.retarget).toHaveBeenCalledWith(undefined);
  });

  it("unsubscribes, aborts the loop signal, and suppresses late retargets on dispose", async () => {
    const unsubscribeCapabilities = vi.fn();
    const unsubscribeCwdInputs = vi.fn();
    const cwd =
      deferred<Awaited<ReturnType<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>>>();
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
    cwd.resolve({ status: "resolved", cwd: "/repo" });
    await tick();

    expect(unsubscribeCapabilities).toHaveBeenCalledOnce();
    expect(unsubscribeCwdInputs).toHaveBeenCalledOnce();
    expect(loopSignal?.aborted).toBe(true);
    expect(loop.retarget).not.toHaveBeenCalled();
  });
});
