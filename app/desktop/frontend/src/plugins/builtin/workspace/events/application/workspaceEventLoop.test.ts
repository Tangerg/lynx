import { afterEach, describe, expect, it, vi } from "vitest";
import { createWorkspaceEventLoop } from "./workspaceEventLoop";

afterEach(() => vi.useRealTimers());

describe("workspace event loop", () => {
  it("invalidates all caches when a lossy stream has a sequence gap", async () => {
    const controller = new AbortController();
    const invalidateAll = vi.fn();
    const handled: number[] = [];
    let receivedBoth!: () => void;
    const done = new Promise<void>((resolve) => {
      receivedBoth = resolve;
    });

    const loop = createWorkspaceEventLoop({
      async subscribe({ signal }) {
        return (async function* () {
          yield { type: "skills.changed", sequence: 41 } as const;
          yield { type: "mcp.changed", sequence: 43 } as const;
          await new Promise<void>((resolve) => {
            signal.addEventListener("abort", () => resolve(), { once: true });
          });
        })();
      },
      handleEvent(event) {
        handled.push(event.sequence);
        if (handled.length === 2) receivedBoth();
      },
      invalidateAll,
      reportError: vi.fn(),
    });

    loop.start(controller.signal);
    await done;
    controller.abort();

    expect(handled).toEqual([41, 43]);
    expect(invalidateAll).toHaveBeenCalledTimes(2); // subscribe + detected gap
  });

  it("retargets from an explicit project back to the default workspace", async () => {
    const outer = new AbortController();
    const subscribed: Array<string | undefined> = [];
    let wake!: () => void;
    let reached = new Promise<void>((resolve) => {
      wake = resolve;
    });
    const loop = createWorkspaceEventLoop({
      async subscribe({ cwd, signal }) {
        subscribed.push(cwd);
        wake();
        return (async function* () {
          yield { type: "resync", sequence: subscribed.length } as const;
          await new Promise<void>((resolve) => {
            if (signal.aborted) resolve();
            else signal.addEventListener("abort", () => resolve(), { once: true });
          });
        })();
      },
      handleEvent: vi.fn(),
      invalidateAll: vi.fn(),
      reportError: vi.fn(),
    });

    loop.start(outer.signal);
    await reached;

    reached = new Promise<void>((resolve) => {
      wake = resolve;
    });
    loop.retarget("/repo");
    await reached;

    reached = new Promise<void>((resolve) => {
      wake = resolve;
    });
    loop.retarget(undefined);
    await reached;
    outer.abort();

    expect(subscribed).toEqual([undefined, "/repo", undefined]);
  });

  it("interrupts reconnect backoff when the workspace target changes", async () => {
    vi.useFakeTimers();
    const outer = new AbortController();
    const subscribe = vi.fn().mockRejectedValue(new Error("offline"));
    const loop = createWorkspaceEventLoop({
      subscribe,
      handleEvent: vi.fn(),
      invalidateAll: vi.fn(),
      reportError: vi.fn(),
    });

    loop.start(outer.signal);
    await vi.advanceTimersByTimeAsync(0);
    expect(subscribe).toHaveBeenCalledTimes(1);
    expect(subscribe.mock.calls[0]?.[0].cwd).toBeUndefined();

    loop.retarget("/new-repo");
    await vi.advanceTimersByTimeAsync(0);
    expect(subscribe).toHaveBeenCalledTimes(2);
    expect(subscribe.mock.calls[1]?.[0].cwd).toBe("/new-repo");

    outer.abort();
    await vi.advanceTimersByTimeAsync(0);
  });

  it("does not publish a stale subscription that resolves after retarget", async () => {
    const outer = new AbortController();
    let resolveOld!: (events: AsyncIterable<{ type: "resync"; sequence: number }>) => void;
    const oldOpening = new Promise<AsyncIterable<{ type: "resync"; sequence: number }>>(
      (resolve) => {
        resolveOld = resolve;
      },
    );
    const subscribed: Array<string | undefined> = [];
    const handled: number[] = [];
    const invalidateAll = vi.fn();
    let reachedNew!: () => void;
    const newSubscription = new Promise<void>((resolve) => {
      reachedNew = resolve;
    });
    const loop = createWorkspaceEventLoop({
      async subscribe({ cwd, signal }) {
        subscribed.push(cwd);
        if (subscribed.length === 1) return oldOpening;
        reachedNew();
        return (async function* () {
          yield { type: "resync", sequence: 2 } as const;
          await new Promise<void>((resolve) => {
            if (signal.aborted) resolve();
            else signal.addEventListener("abort", () => resolve(), { once: true });
          });
        })();
      },
      handleEvent: (event) => handled.push(event.sequence),
      invalidateAll,
      reportError: vi.fn(),
    });

    loop.start(outer.signal);
    await Promise.resolve();
    loop.retarget("/new-repo");
    resolveOld(
      (async function* () {
        yield { type: "resync", sequence: 1 } as const;
      })(),
    );
    await newSubscription;
    await vi.waitFor(() => expect(handled).toEqual([2]));
    outer.abort();

    expect(subscribed).toEqual([undefined, "/new-repo"]);
    expect(invalidateAll).toHaveBeenCalledTimes(1);
  });
});
