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
          yield { type: "skills.changed", sequence: 1 } as const;
          yield { type: "mcp.changed", sequence: 3 } as const;
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

    expect(handled).toEqual([1, 3]);
    expect(invalidateAll).toHaveBeenCalledTimes(2); // subscribe + detected gap
  });

  it("invalidates all caches when the first frame is not sequence one", async () => {
    const controller = new AbortController();
    const invalidateAll = vi.fn();
    let handled!: () => void;
    const received = new Promise<void>((resolve) => {
      handled = resolve;
    });
    const loop = createWorkspaceEventLoop({
      async subscribe({ signal }) {
        return (async function* () {
          yield { type: "skills.changed", sequence: 2 } as const;
          await new Promise<void>((resolve) => {
            signal.addEventListener("abort", () => resolve(), { once: true });
          });
        })();
      },
      handleEvent: handled,
      invalidateAll,
      reportError: vi.fn(),
    });

    loop.start(controller.signal);
    await received;
    controller.abort();

    expect(invalidateAll).toHaveBeenCalledTimes(2); // subscribe + missing sequence 1
  });

  it("keeps unresolved identity distinct from the default workspace", async () => {
    const outer = new AbortController();
    const subscribed: Array<{ type: "none" } | { type: "workspace"; cwd?: string }> = [];
    let wake!: () => void;
    let reached = new Promise<void>((resolve) => {
      wake = resolve;
    });
    const loop = createWorkspaceEventLoop({
      async subscribe({ target, signal }) {
        subscribed.push(target);
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
    loop.retarget({ type: "workspace", cwd: "/repo" });
    await reached;

    reached = new Promise<void>((resolve) => {
      wake = resolve;
    });
    loop.retarget({ type: "workspace" });
    await reached;
    outer.abort();

    expect(subscribed).toEqual([
      { type: "none" },
      { type: "workspace", cwd: "/repo" },
      { type: "workspace" },
    ]);
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
    expect(subscribe.mock.calls[0]?.[0].target).toEqual({ type: "none" });

    loop.retarget({ type: "workspace", cwd: "/new-repo" });
    await vi.advanceTimersByTimeAsync(0);
    expect(subscribe).toHaveBeenCalledTimes(2);
    expect(subscribe.mock.calls[1]?.[0].target).toEqual({
      type: "workspace",
      cwd: "/new-repo",
    });

    outer.abort();
    await vi.advanceTimersByTimeAsync(0);
  });

  it("cancels an in-flight subscription opening before retargeting", async () => {
    const outer = new AbortController();
    const subscribed: Array<{ type: "none" } | { type: "workspace"; cwd?: string }> = [];
    let reachedNew!: () => void;
    const newSubscription = new Promise<void>((resolve) => {
      reachedNew = resolve;
    });
    const loop = createWorkspaceEventLoop({
      subscribe({ target, signal }) {
        subscribed.push(target);
        if (subscribed.length === 1) {
          return new Promise((_, reject) => {
            signal.addEventListener("abort", () => reject(signal.reason), { once: true });
          });
        }
        reachedNew();
        return Promise.resolve(
          (async function* () {
            await new Promise<void>((resolve) => {
              if (signal.aborted) resolve();
              else signal.addEventListener("abort", () => resolve(), { once: true });
            });
            yield* [];
          })(),
        );
      },
      handleEvent: vi.fn(),
      invalidateAll: vi.fn(),
      reportError: vi.fn(),
    });

    loop.start(outer.signal);
    await Promise.resolve();
    loop.retarget({ type: "workspace", cwd: "/new-repo" });
    await newSubscription;
    outer.abort();

    expect(subscribed).toEqual([{ type: "none" }, { type: "workspace", cwd: "/new-repo" }]);
  });

  it("does not publish a stale subscription that resolves after retarget", async () => {
    const outer = new AbortController();
    let resolveOld!: (events: AsyncIterable<{ type: "resync"; sequence: number }>) => void;
    const oldOpening = new Promise<AsyncIterable<{ type: "resync"; sequence: number }>>(
      (resolve) => {
        resolveOld = resolve;
      },
    );
    const subscribed: Array<{ type: "none" } | { type: "workspace"; cwd?: string }> = [];
    const handled: number[] = [];
    const invalidateAll = vi.fn();
    let reachedNew!: () => void;
    const newSubscription = new Promise<void>((resolve) => {
      reachedNew = resolve;
    });
    const loop = createWorkspaceEventLoop({
      async subscribe({ target, signal }) {
        subscribed.push(target);
        if (subscribed.length === 1) return oldOpening;
        reachedNew();
        return (async function* () {
          // Sequence is connection-local and restarts at one after retarget.
          yield { type: "resync", sequence: 1 } as const;
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
    loop.retarget({ type: "workspace", cwd: "/new-repo" });
    resolveOld(
      (async function* () {
        yield { type: "resync", sequence: 1 } as const;
      })(),
    );
    await newSubscription;
    await vi.waitFor(() => expect(handled).toEqual([1]));
    outer.abort();

    expect(subscribed).toEqual([{ type: "none" }, { type: "workspace", cwd: "/new-repo" }]);
    expect(invalidateAll).toHaveBeenCalledTimes(1);
  });
});
