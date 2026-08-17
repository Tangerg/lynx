import { afterEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceEventLoop } from "./workspaceEventLoop";
import {
  startWorkspaceEventSubscription,
  type WorkspaceCwdInputChange,
  type WorkspaceCwdResolution,
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

afterEach(() => vi.useRealTimers());

function subscriptionPorts(
  patch: Partial<WorkspaceEventSubscriptionPorts> = {},
): WorkspaceEventSubscriptionPorts {
  const loop: WorkspaceEventLoop = {
    start: vi.fn(async () => {}),
    retarget: vi.fn(),
  };
  return {
    canSubscribe: vi.fn(() => true),
    connectionGeneration: vi.fn(() => "runtime_1"),
    subscribeConnection: vi.fn(() => vi.fn()),
    retireReadModels: vi.fn(),
    resolveWorkspaceCwd: vi.fn().mockResolvedValue({ status: "resolved", cwd: "/repo" }),
    reportResolutionError: vi.fn(),
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
    expect(ports.loop.start).toHaveBeenCalledWith(expect.any(AbortSignal), "runtime_1");
  });

  it("starts once when the capability is advertised later", () => {
    let onConnectionChange: (() => void) | undefined;
    let advertised = false;
    const ports = subscriptionPorts({
      canSubscribe: () => advertised,
      subscribeConnection: (listener) => {
        onConnectionChange = listener;
        return vi.fn();
      },
    });

    startWorkspaceEventSubscription(ports);
    expect(ports.loop.start).not.toHaveBeenCalled();

    advertised = true;
    onConnectionChange?.();
    onConnectionChange?.();

    expect(ports.loop.start).toHaveBeenCalledOnce();
  });

  it("stops and reopens the event loop when discovery withdraws and restores streaming", () => {
    let onConnectionChange: (() => void) | undefined;
    let advertised = true;
    const signals: AbortSignal[] = [];
    const ports = subscriptionPorts({
      canSubscribe: () => advertised,
      subscribeConnection: (listener) => {
        onConnectionChange = listener;
        return vi.fn();
      },
      loop: {
        start: vi.fn(async (signal) => {
          signals.push(signal);
        }),
        retarget: vi.fn(),
      },
    });

    startWorkspaceEventSubscription(ports);
    expect(signals).toHaveLength(1);
    expect(signals[0]?.aborted).toBe(false);

    advertised = false;
    onConnectionChange?.();
    expect(signals[0]?.aborted).toBe(true);

    advertised = true;
    onConnectionChange?.();
    expect(signals).toHaveLength(2);
    expect(signals[1]?.aborted).toBe(false);
  });

  it("supersedes the event loop when a ready Runtime is replaced in place", () => {
    let onConnectionChange: (() => void) | undefined;
    let generation = "runtime_retired";
    const signals: AbortSignal[] = [];
    const ports = subscriptionPorts({
      connectionGeneration: () => generation,
      subscribeConnection: (listener) => {
        onConnectionChange = listener;
        return vi.fn();
      },
      loop: {
        start: vi.fn(async (signal) => {
          signals.push(signal);
        }),
        retarget: vi.fn(),
      },
    });

    startWorkspaceEventSubscription(ports);
    expect(signals).toHaveLength(1);

    onConnectionChange?.();
    expect(signals).toHaveLength(1);
    expect(signals[0]?.aborted).toBe(false);

    generation = "runtime_successor";
    onConnectionChange?.();
    expect(signals).toHaveLength(2);
    expect(signals[0]?.aborted).toBe(true);
    expect(signals[1]?.aborted).toBe(false);
  });

  it("retires old read admissions before opening a successor Runtime event tail", () => {
    let onConnectionChange: (() => void) | undefined;
    let generation = "runtime_retired";
    const order: string[] = [];
    const ports = subscriptionPorts({
      connectionGeneration: () => generation,
      subscribeConnection: (listener) => {
        onConnectionChange = listener;
        return vi.fn();
      },
      retireReadModels: () => order.push("retire"),
      loop: {
        start: vi.fn(async () => {
          order.push("start");
        }),
        retarget: vi.fn(),
      },
    });

    startWorkspaceEventSubscription(ports);
    order.length = 0;
    generation = "runtime_successor";
    onConnectionChange?.();

    expect(order).toEqual(["retire", "start"]);
  });

  it("retires the exact disconnected generation before admitting its recovered tail", () => {
    let onConnectionChange: (() => void) | undefined;
    let generation: string | null = "connection_retired";
    const order: string[] = [];
    const signals: AbortSignal[] = [];
    const ports = subscriptionPorts({
      connectionGeneration: () => generation,
      subscribeConnection: (listener) => {
        onConnectionChange = listener;
        return vi.fn();
      },
      retireReadModels: () => order.push("retire"),
      loop: {
        start: vi.fn(async (signal, connectionGeneration) => {
          signals.push(signal);
          order.push(`start:${connectionGeneration}`);
        }),
        retarget: vi.fn(),
      },
    });

    startWorkspaceEventSubscription(ports);
    order.length = 0;
    generation = null;
    onConnectionChange?.();

    expect(signals[0]?.aborted).toBe(true);
    expect(order).toEqual(["retire"]);

    generation = "connection_recovered";
    onConnectionChange?.();
    expect(order).toEqual(["retire", "retire", "start:connection_recovered"]);
    expect(signals[1]?.aborted).toBe(false);
  });

  it("retires reads admitted while disconnected before the first recovered tail", () => {
    let onConnectionChange: (() => void) | undefined;
    let generation: string | null = null;
    let advertised = false;
    const order: string[] = [];
    const ports = subscriptionPorts({
      canSubscribe: () => advertised,
      connectionGeneration: () => generation,
      subscribeConnection: (listener) => {
        onConnectionChange = listener;
        return vi.fn();
      },
      retireReadModels: () => order.push("retire"),
      loop: {
        start: vi.fn(async () => {
          order.push("start");
        }),
        retarget: vi.fn(),
      },
    });

    startWorkspaceEventSubscription(ports);
    generation = "runtime_recovered";
    advertised = true;
    onConnectionChange?.();

    expect(order).toEqual(["retire", "start"]);
  });

  it("retargets to the latest resolved cwd and ignores stale resolutions", async () => {
    const first =
      deferred<Awaited<ReturnType<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>>>();
    const second =
      deferred<Awaited<ReturnType<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>>>();
    let onCwdChange: ((change: WorkspaceCwdInputChange) => void) | undefined;
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
    onCwdChange?.("identity");
    first.resolve({ status: "resolved", cwd: "/old" });
    await tick();
    second.resolve({ status: "resolved", cwd: "/new" });
    await tick();

    expect(ports.loop.retarget).toHaveBeenNthCalledWith(1, { type: "none" });
    expect(ports.loop.retarget).toHaveBeenNthCalledWith(2, { type: "none" });
    expect(ports.loop.retarget).toHaveBeenNthCalledWith(3, {
      type: "workspace",
      cwd: "/new",
    });
  });

  it("aborts in-flight identity reads when retargeted or disposed", async () => {
    let onCwdChange: ((change: WorkspaceCwdInputChange) => void) | undefined;
    const signals: AbortSignal[] = [];
    const resolveWorkspaceCwd = vi.fn<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>(
      (signal) => {
        signals.push(signal);
        return new Promise<WorkspaceCwdResolution>((_resolve, reject) => {
          signal.addEventListener(
            "abort",
            () => reject(signal.reason ?? new DOMException("Aborted", "AbortError")),
            { once: true },
          );
        });
      },
    );
    const ports = subscriptionPorts({
      resolveWorkspaceCwd,
      subscribeWorkspaceCwdInputs: (listener) => {
        onCwdChange = listener;
        return vi.fn();
      },
    });

    const dispose = startWorkspaceEventSubscription(ports);
    expect(signals).toHaveLength(1);
    expect(signals[0]).toBeInstanceOf(AbortSignal);

    onCwdChange?.("identity");
    expect(signals[0]?.aborted).toBe(true);
    expect(signals).toHaveLength(2);
    expect(signals[1]).toBeInstanceOf(AbortSignal);

    dispose();
    expect(signals[1]?.aborted).toBe(true);
    await tick();
    expect(ports.reportResolutionError).not.toHaveBeenCalled();
  });

  it("keeps global topics online until a new identity input resolves the workspace", async () => {
    let onCwdChange: ((change: WorkspaceCwdInputChange) => void) | undefined;
    const ports = subscriptionPorts({
      resolveWorkspaceCwd: vi
        .fn<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>()
        .mockResolvedValueOnce({ status: "unavailable" })
        .mockResolvedValueOnce({ status: "resolved" }),
      subscribeWorkspaceCwdInputs: (listener) => {
        onCwdChange = listener;
        return vi.fn();
      },
    });

    startWorkspaceEventSubscription(ports);
    await tick();
    expect(ports.loop.retarget).toHaveBeenCalledTimes(2);
    expect(ports.loop.retarget).toHaveBeenLastCalledWith({ type: "none" });
    expect(ports.resolveWorkspaceCwd).toHaveBeenCalledOnce();

    onCwdChange?.("identity");
    await tick();
    expect(ports.resolveWorkspaceCwd).toHaveBeenCalledTimes(2);
    expect(ports.loop.retarget).toHaveBeenLastCalledWith({ type: "workspace" });
  });

  it("retries a transient workspace resolution without waiting for another identity change", async () => {
    vi.useFakeTimers();
    const resolveWorkspaceCwd = vi
      .fn<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValue({ status: "resolved", cwd: "/recovered" });
    const ports = subscriptionPorts({ resolveWorkspaceCwd });

    const dispose = startWorkspaceEventSubscription(ports);
    await vi.advanceTimersByTimeAsync(0);
    expect(resolveWorkspaceCwd).toHaveBeenCalledOnce();
    expect(ports.loop.retarget).toHaveBeenLastCalledWith({ type: "none" });
    expect(ports.reportResolutionError).toHaveBeenCalledOnce();

    await vi.advanceTimersByTimeAsync(999);
    expect(resolveWorkspaceCwd).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(1);
    expect(resolveWorkspaceCwd).toHaveBeenCalledTimes(2);
    expect(ports.loop.retarget).toHaveBeenLastCalledWith({
      type: "workspace",
      cwd: "/recovered",
    });
    dispose();
  });

  it("cancels a failed identity's backoff when the active session changes", async () => {
    vi.useFakeTimers();
    let onCwdChange: ((change: WorkspaceCwdInputChange) => void) | undefined;
    const resolveWorkspaceCwd = vi
      .fn<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>()
      .mockRejectedValueOnce(new Error("old session offline"))
      .mockResolvedValue({ status: "resolved", cwd: "/new-session" });
    const ports = subscriptionPorts({
      resolveWorkspaceCwd,
      subscribeWorkspaceCwdInputs: (listener) => {
        onCwdChange = listener;
        return vi.fn();
      },
    });

    const dispose = startWorkspaceEventSubscription(ports);
    await vi.advanceTimersByTimeAsync(0);
    onCwdChange?.("identity");
    await vi.advanceTimersByTimeAsync(0);

    expect(resolveWorkspaceCwd).toHaveBeenCalledTimes(2);
    expect(ports.loop.retarget).toHaveBeenLastCalledWith({
      type: "workspace",
      cwd: "/new-session",
    });
    await vi.advanceTimersByTimeAsync(1_000);
    expect(resolveWorkspaceCwd).toHaveBeenCalledTimes(2);
    dispose();
  });

  it("caps repeated resolution backoff at thirty seconds", async () => {
    vi.useFakeTimers();
    const resolveWorkspaceCwd = vi
      .fn<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>()
      .mockRejectedValue(new Error("offline"));
    const ports = subscriptionPorts({ resolveWorkspaceCwd });

    const dispose = startWorkspaceEventSubscription(ports);
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(60_999);
    expect(resolveWorkspaceCwd).toHaveBeenCalledTimes(6);

    await vi.advanceTimersByTimeAsync(1);
    expect(resolveWorkspaceCwd).toHaveBeenCalledTimes(7);
    await vi.advanceTimersByTimeAsync(29_999);
    expect(resolveWorkspaceCwd).toHaveBeenCalledTimes(7);
    await vi.advanceTimersByTimeAsync(1);
    expect(resolveWorkspaceCwd).toHaveBeenCalledTimes(8);
    dispose();
  });

  it("cancels workspace resolution backoff on dispose", async () => {
    vi.useFakeTimers();
    const resolveWorkspaceCwd = vi
      .fn<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>()
      .mockRejectedValue(new Error("offline"));
    const ports = subscriptionPorts({ resolveWorkspaceCwd });

    const dispose = startWorkspaceEventSubscription(ports);
    await vi.advanceTimersByTimeAsync(0);
    dispose();
    await vi.advanceTimersByTimeAsync(30_000);

    expect(resolveWorkspaceCwd).toHaveBeenCalledOnce();
  });

  it("drops the previous file watch before resolving a newly selected session", async () => {
    let onCwdChange: ((change: WorkspaceCwdInputChange) => void) | undefined;
    const second =
      deferred<Awaited<ReturnType<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>>>();
    const ports = subscriptionPorts({
      resolveWorkspaceCwd: vi
        .fn<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>()
        .mockResolvedValueOnce({ status: "resolved", cwd: "/first" })
        .mockReturnValueOnce(second.promise),
      subscribeWorkspaceCwdInputs: (listener) => {
        onCwdChange = listener;
        return vi.fn();
      },
    });

    startWorkspaceEventSubscription(ports);
    await tick();
    expect(ports.loop.retarget).toHaveBeenLastCalledWith({
      type: "workspace",
      cwd: "/first",
    });

    onCwdChange?.("identity");
    expect(ports.loop.retarget).toHaveBeenLastCalledWith({ type: "none" });
    second.resolve({ status: "unavailable" });
    await tick();
    expect(ports.loop.retarget).toHaveBeenLastCalledWith({ type: "none" });
  });

  it("keeps an active workspace watch while its session projection catches up", async () => {
    let onCwdChange: ((change: WorkspaceCwdInputChange) => void) | undefined;
    const ports = subscriptionPorts({
      resolveWorkspaceCwd: vi
        .fn<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>()
        .mockResolvedValue({ status: "resolved", cwd: "/repo" }),
      subscribeWorkspaceCwdInputs: (listener) => {
        onCwdChange = listener;
        return vi.fn();
      },
    });

    startWorkspaceEventSubscription(ports);
    await tick();
    onCwdChange?.("projection");
    await tick();

    expect(ports.loop.retarget).toHaveBeenNthCalledWith(1, { type: "none" });
    expect(ports.loop.retarget).toHaveBeenNthCalledWith(2, {
      type: "workspace",
      cwd: "/repo",
    });
    expect(ports.loop.retarget).toHaveBeenNthCalledWith(3, {
      type: "workspace",
      cwd: "/repo",
    });
  });

  it("withdraws an active workspace watch when its projection becomes unavailable", async () => {
    let onCwdChange: ((change: WorkspaceCwdInputChange) => void) | undefined;
    const ports = subscriptionPorts({
      resolveWorkspaceCwd: vi
        .fn<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>()
        .mockResolvedValueOnce({ status: "resolved", cwd: "/repo" })
        .mockResolvedValueOnce({ status: "unavailable" }),
      subscribeWorkspaceCwdInputs: (listener) => {
        onCwdChange = listener;
        return vi.fn();
      },
    });

    startWorkspaceEventSubscription(ports);
    await tick();
    onCwdChange?.("projection");
    await tick();

    expect(ports.loop.retarget).toHaveBeenLastCalledWith({ type: "none" });
  });

  it("unsubscribes, aborts the loop signal, and suppresses late retargets on dispose", async () => {
    const unsubscribeConnection = vi.fn();
    const unsubscribeCwdInputs = vi.fn();
    const cwd =
      deferred<Awaited<ReturnType<WorkspaceEventSubscriptionPorts["resolveWorkspaceCwd"]>>>();
    let loopSignal: AbortSignal | undefined;
    const loop: WorkspaceEventLoop = {
      start: vi.fn(async (signal) => {
        loopSignal = signal;
      }),
      retarget: vi.fn(),
    };
    const ports = subscriptionPorts({
      loop,
      resolveWorkspaceCwd: vi.fn().mockReturnValue(cwd.promise),
      subscribeConnection: vi.fn(() => unsubscribeConnection),
      subscribeWorkspaceCwdInputs: vi.fn(() => unsubscribeCwdInputs),
    });

    const dispose = startWorkspaceEventSubscription(ports);
    dispose();
    cwd.resolve({ status: "resolved", cwd: "/repo" });
    await tick();

    expect(unsubscribeConnection).toHaveBeenCalledOnce();
    expect(unsubscribeCwdInputs).toHaveBeenCalledOnce();
    expect(loopSignal?.aborted).toBe(true);
    expect(loop.retarget).toHaveBeenCalledOnce();
    expect(loop.retarget).toHaveBeenCalledWith({ type: "none" });
  });
});
