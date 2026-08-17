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
      reportDisconnect: vi.fn(),
    });

    const run = loop.start(controller.signal, "connection_1");
    await done;
    controller.abort();
    await run;

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
      reportDisconnect: vi.fn(),
    });

    const run = loop.start(controller.signal, "connection_1");
    await received;
    controller.abort();
    await run;

    expect(invalidateAll).toHaveBeenCalledTimes(2); // subscribe + missing sequence 1
  });

  it("drops a duplicated frame without replacing every read model again", async () => {
    const controller = new AbortController();
    const invalidateAll = vi.fn();
    const handled: Array<{ type: string; sequence: number }> = [];
    let received!: () => void;
    const done = new Promise<void>((resolve) => {
      received = resolve;
    });
    const loop = createWorkspaceEventLoop({
      async subscribe({ signal }) {
        return (async function* () {
          yield { type: "skills.changed", sequence: 1 } as const;
          yield { type: "goals.changed", sequence: 1 } as const;
          yield { type: "runs.changed", sequence: 2 } as const;
          await new Promise<void>((resolve) => {
            signal.addEventListener("abort", () => resolve(), { once: true });
          });
        })();
      },
      handleEvent(event) {
        handled.push({ type: event.type, sequence: event.sequence });
        if (event.sequence === 2) received();
      },
      invalidateAll,
      reportDisconnect: vi.fn(),
    });

    const run = loop.start(controller.signal, "connection_1");
    await done;
    controller.abort();
    await run;

    expect(handled).toEqual([
      { type: "skills.changed", sequence: 1 },
      { type: "runs.changed", sequence: 2 },
    ]);
    expect(invalidateAll).toHaveBeenCalledOnce();
  });

  it("keeps its watermark monotonic when a missing frame arrives after the gap", async () => {
    const controller = new AbortController();
    const invalidateAll = vi.fn();
    const handled: number[] = [];
    let received!: () => void;
    const done = new Promise<void>((resolve) => {
      received = resolve;
    });
    const loop = createWorkspaceEventLoop({
      async subscribe({ signal }) {
        return (async function* () {
          yield { type: "skills.changed", sequence: 1 } as const;
          yield { type: "runs.changed", sequence: 3 } as const;
          yield { type: "goals.changed", sequence: 2 } as const;
          yield { type: "interrupts.changed", sequence: 4 } as const;
          await new Promise<void>((resolve) => {
            signal.addEventListener("abort", () => resolve(), { once: true });
          });
        })();
      },
      handleEvent(event) {
        handled.push(event.sequence);
        if (event.sequence === 4) received();
      },
      invalidateAll,
      reportDisconnect: vi.fn(),
    });

    const run = loop.start(controller.signal, "connection_1");
    await done;
    controller.abort();
    await run;

    expect(handled).toEqual([1, 3, 4]);
    expect(invalidateAll).toHaveBeenCalledTimes(2); // subscribe + the forward gap
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
      reportDisconnect: vi.fn(),
    });

    const run = loop.start(outer.signal, "connection_1");
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
    await run;

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
    const reportDisconnect = vi.fn();
    const loop = createWorkspaceEventLoop({
      subscribe,
      handleEvent: vi.fn(),
      invalidateAll: vi.fn(),
      reportDisconnect,
    });

    const run = loop.start(outer.signal, "connection_1");
    await vi.advanceTimersByTimeAsync(0);
    expect(subscribe).toHaveBeenCalledTimes(1);
    expect(reportDisconnect).toHaveBeenCalledOnce();
    expect(reportDisconnect).toHaveBeenCalledWith(
      "connection_1",
      expect.objectContaining({ message: "offline" }),
    );
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
    await run;
  });

  it("reports a clean remote stream end as a connection signal", async () => {
    vi.useFakeTimers();
    const outer = new AbortController();
    const reportDisconnect = vi.fn();
    const loop = createWorkspaceEventLoop({
      subscribe: vi.fn().mockResolvedValue(
        (async function* () {
          yield* [];
        })(),
      ),
      handleEvent: vi.fn(),
      invalidateAll: vi.fn(),
      reportDisconnect,
    });

    const run = loop.start(outer.signal, "connection_1");
    await vi.advanceTimersByTimeAsync(0);

    expect(reportDisconnect).toHaveBeenCalledOnce();
    expect(reportDisconnect).toHaveBeenCalledWith("connection_1", undefined);
    outer.abort();
    await vi.advanceTimersByTimeAsync(0);
    await run;
  });

  it("does not retain a reconnect timer after connection withdrawal aborts its generation", async () => {
    vi.useFakeTimers();
    const outer = new AbortController();
    const subscribe = vi.fn().mockResolvedValue(
      (async function* () {
        yield* [];
      })(),
    );
    const loop = createWorkspaceEventLoop({
      subscribe,
      handleEvent: vi.fn(),
      invalidateAll: vi.fn(),
      reportDisconnect: () => outer.abort(),
    });

    const run = loop.start(outer.signal, "connection_retired");
    await vi.advanceTimersByTimeAsync(0);
    await run;

    expect(subscribe).toHaveBeenCalledOnce();
    expect(vi.getTimerCount()).toBe(0);
  });

  it("terminates a subscription opening that never settles", async () => {
    vi.useFakeTimers();
    const outer = new AbortController();
    let openingSignal: AbortSignal | undefined;
    let resolveOpening!: (events: AsyncIterable<{ type: "resync"; sequence: number }>) => void;
    const opening = new Promise<AsyncIterable<{ type: "resync"; sequence: number }>>((resolve) => {
      resolveOpening = resolve;
    });
    const closeLateStream = vi.fn(async () => ({ value: undefined, done: true }) as const);
    const reportDisconnect = vi.fn();
    const loop = createWorkspaceEventLoop({
      subscribe: vi.fn(({ signal }) => {
        openingSignal = signal;
        return opening;
      }),
      handleEvent: vi.fn(),
      invalidateAll: vi.fn(),
      reportDisconnect,
      openingTimeoutMs: 50,
    });

    const run = loop.start(outer.signal, "connection_1");
    await vi.advanceTimersByTimeAsync(49);
    expect(reportDisconnect).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1);
    expect(openingSignal?.aborted).toBe(true);
    expect(reportDisconnect).toHaveBeenCalledWith(
      "connection_1",
      expect.objectContaining({
        name: "WorkspaceEventOpeningTimeoutError",
        message: "runtime_event_subscription_opening_timeout",
      }),
    );
    resolveOpening({
      [Symbol.asyncIterator]: () => ({
        next: vi.fn(),
        return: closeLateStream,
      }),
    });
    await vi.waitFor(() => expect(closeLateStream).toHaveBeenCalledOnce());

    outer.abort();
    await vi.runAllTimersAsync();
    await run;
  });

  it("releases the opening deadline after the event tail is accepted", async () => {
    vi.useFakeTimers();
    const outer = new AbortController();
    let streamSignal: AbortSignal | undefined;
    const invalidateAll = vi.fn();
    const reportDisconnect = vi.fn();
    const loop = createWorkspaceEventLoop({
      async subscribe({ signal }) {
        streamSignal = signal;
        return (async function* () {
          await new Promise<void>((resolve) => {
            signal.addEventListener("abort", () => resolve(), { once: true });
          });
          yield* [];
        })();
      },
      handleEvent: vi.fn(),
      invalidateAll,
      reportDisconnect,
      openingTimeoutMs: 50,
    });

    const run = loop.start(outer.signal, "connection_1");
    await vi.advanceTimersByTimeAsync(0);
    expect(invalidateAll).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(500);

    expect(streamSignal?.aborted).toBe(false);
    expect(reportDisconnect).not.toHaveBeenCalled();
    outer.abort();
    await vi.runAllTimersAsync();
    await run;
  });

  it("recovers from a timed-out opening through the normal reconnect owner", async () => {
    vi.useFakeTimers();
    const outer = new AbortController();
    let resolveRetired!: (events: AsyncIterable<{ type: "resync"; sequence: number }>) => void;
    const retired = new Promise<AsyncIterable<{ type: "resync"; sequence: number }>>((resolve) => {
      resolveRetired = resolve;
    });
    let calls = 0;
    const invalidateAll = vi.fn();
    const reportDisconnect = vi.fn();
    const loop = createWorkspaceEventLoop({
      subscribe: vi.fn(({ signal }) => {
        calls += 1;
        if (calls === 1) return retired;
        return Promise.resolve(
          (async function* () {
            await new Promise<void>((resolve) => {
              signal.addEventListener("abort", () => resolve(), { once: true });
            });
            yield* [];
          })(),
        );
      }),
      handleEvent: vi.fn(),
      invalidateAll,
      reportDisconnect,
      openingTimeoutMs: 50,
    });

    const run = loop.start(outer.signal, "connection_1");
    await vi.advanceTimersByTimeAsync(50);
    expect(reportDisconnect).toHaveBeenCalledOnce();
    resolveRetired(
      (async function* () {
        yield* [];
      })(),
    );

    await vi.advanceTimersByTimeAsync(1_000);
    expect(calls).toBe(2);
    expect(invalidateAll).toHaveBeenCalledOnce();

    outer.abort();
    await vi.runAllTimersAsync();
    await run;
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
      reportDisconnect: vi.fn(),
    });

    const run = loop.start(outer.signal, "connection_1");
    await Promise.resolve();
    loop.retarget({ type: "workspace", cwd: "/new-repo" });
    await newSubscription;
    outer.abort();
    await run;

    expect(subscribed).toEqual([{ type: "none" }, { type: "workspace", cwd: "/new-repo" }]);
  });

  it("retargets without waiting for an opening that ignores cancellation", async () => {
    const outer = new AbortController();
    const subscribed: Array<{ type: "none" } | { type: "workspace"; cwd?: string }> = [];
    let resolveOld!: (events: AsyncIterable<{ type: "resync"; sequence: number }>) => void;
    const oldOpening = new Promise<AsyncIterable<{ type: "resync"; sequence: number }>>(
      (resolve) => {
        resolveOld = resolve;
      },
    );
    const closeOld = vi.fn(async () => ({ value: undefined, done: true }) as const);
    const loop = createWorkspaceEventLoop({
      async subscribe({ target, signal }) {
        subscribed.push(target);
        if (subscribed.length === 1) return oldOpening;
        return (async function* () {
          await new Promise<void>((resolve) => {
            if (signal.aborted) resolve();
            else signal.addEventListener("abort", () => resolve(), { once: true });
          });
          yield* [];
        })();
      },
      handleEvent: vi.fn(),
      invalidateAll: vi.fn(),
      reportDisconnect: vi.fn(),
    });

    const run = loop.start(outer.signal, "connection_1");
    await vi.waitFor(() => expect(subscribed).toHaveLength(1));
    loop.retarget({ type: "workspace", cwd: "/new-repo" });
    try {
      await vi.waitFor(() => expect(subscribed).toHaveLength(2), { timeout: 100 });
      resolveOld({
        [Symbol.asyncIterator]: () => ({
          next: async () => ({ value: undefined, done: true }) as const,
          return: closeOld,
        }),
      });
      await vi.waitFor(() => expect(closeOld).toHaveBeenCalledOnce());
    } finally {
      outer.abort();
      resolveOld(
        (async function* () {
          yield* [];
        })(),
      );
      await run;
    }

    expect(subscribed).toEqual([{ type: "none" }, { type: "workspace", cwd: "/new-repo" }]);
  });

  it("retargets when the active iterator ignores cancellation", async () => {
    const outer = new AbortController();
    const subscribed: Array<{ type: "none" } | { type: "workspace"; cwd?: string }> = [];
    let releaseNext!: (result: IteratorResult<{ type: "resync"; sequence: number }>) => void;
    const closeOld = vi.fn(async () => ({ value: undefined, done: true }) as const);
    const loop = createWorkspaceEventLoop({
      async subscribe({ target, signal }) {
        subscribed.push(target);
        if (subscribed.length === 1) {
          return {
            [Symbol.asyncIterator]: () => ({
              next: () =>
                new Promise<IteratorResult<{ type: "resync"; sequence: number }>>((resolve) => {
                  releaseNext = resolve;
                }),
              return: closeOld,
            }),
          };
        }
        return (async function* () {
          await new Promise<void>((resolve) => {
            if (signal.aborted) resolve();
            else signal.addEventListener("abort", () => resolve(), { once: true });
          });
          yield* [];
        })();
      },
      handleEvent: vi.fn(),
      invalidateAll: vi.fn(),
      reportDisconnect: vi.fn(),
    });

    const run = loop.start(outer.signal, "connection_1");
    await vi.waitFor(() => expect(releaseNext).toBeTypeOf("function"));
    loop.retarget({ type: "workspace", cwd: "/new-repo" });
    try {
      await vi.waitFor(() => expect(subscribed).toHaveLength(2), { timeout: 100 });
      expect(closeOld).toHaveBeenCalledOnce();
    } finally {
      releaseNext({ value: undefined, done: true });
      outer.abort();
      await run;
    }

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
      reportDisconnect: vi.fn(),
    });

    const run = loop.start(outer.signal, "connection_1");
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
    await run;

    expect(subscribed).toEqual([{ type: "none" }, { type: "workspace", cwd: "/new-repo" }]);
    expect(invalidateAll).toHaveBeenCalledTimes(1);
  });

  it("keeps a restarted generation from losing retarget ownership to the stopped loop", async () => {
    const first = new AbortController();
    const second = new AbortController();
    const subscribed: string[] = [];
    let reachedSecond!: () => void;
    const secondOpened = new Promise<void>((resolve) => {
      reachedSecond = resolve;
    });
    const loop = createWorkspaceEventLoop({
      async subscribe({ target, signal }) {
        subscribed.push(target.type === "none" ? "none" : (target.cwd ?? "default"));
        if (subscribed.length === 2) reachedSecond();
        return (async function* () {
          await new Promise<void>((resolve) => {
            if (signal.aborted) resolve();
            else signal.addEventListener("abort", () => resolve(), { once: true });
          });
          yield* [];
        })();
      },
      handleEvent: vi.fn(),
      invalidateAll: vi.fn(),
      reportDisconnect: vi.fn(),
    });

    const firstRun = loop.start(first.signal, "connection_1");
    await Promise.resolve();
    first.abort();
    const secondRun = loop.start(second.signal, "connection_2");
    await secondOpened;
    loop.retarget({ type: "workspace", cwd: "/recovered" });
    await vi.waitFor(() => expect(subscribed).toContain("/recovered"));
    second.abort();
    await Promise.all([firstRun, secondRun]);

    expect(subscribed).toEqual(["none", "none", "/recovered"]);
  });

  it("atomically replaces the active generation even when its caller did not abort", async () => {
    const first = new AbortController();
    const second = new AbortController();
    const subscriptionSignals: AbortSignal[] = [];
    let openedTwice!: () => void;
    const twice = new Promise<void>((resolve) => {
      openedTwice = resolve;
    });
    const loop = createWorkspaceEventLoop({
      async subscribe({ signal }) {
        subscriptionSignals.push(signal);
        if (subscriptionSignals.length === 2) openedTwice();
        return (async function* () {
          await new Promise<void>((resolve) => {
            if (signal.aborted) resolve();
            else signal.addEventListener("abort", () => resolve(), { once: true });
          });
          yield* [];
        })();
      },
      handleEvent: vi.fn(),
      invalidateAll: vi.fn(),
      reportDisconnect: vi.fn(),
    });

    const firstRun = loop.start(first.signal, "connection_1");
    await vi.waitFor(() => expect(subscriptionSignals).toHaveLength(1));
    const secondRun = loop.start(second.signal, "connection_2");
    await twice;

    expect(first.signal.aborted).toBe(false);
    expect(subscriptionSignals[0]?.aborted).toBe(true);
    expect(subscriptionSignals[1]?.aborted).toBe(false);
    second.abort();
    await Promise.all([firstRun, secondRun]);
  });
});
