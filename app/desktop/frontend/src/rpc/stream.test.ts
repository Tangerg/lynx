// Run-event stream lifecycle (API.md §5 / §10). The headline guarantees:
//   - the stream ends on the ROOT SEGMENT's `segment.finished` (no separate
//     "closed" method in v2);
//   - a single root-segment stream carries the whole run tree (subagent runs
//     are admitted via spawnedByItemId, keyed on their distinct runIds);
//   - the root segment id is bound from the call response, AFTER the eager
//     subscription (so head events under streamable HTTP aren't dropped);
//   - the client subscription is torn down on BOTH natural completion and
//     early break (otherwise every finished run leaks a subscriber).

import type { NotificationObserver, RpcClient, StreamEndHandler } from "./client";
import { describe, expect, it } from "vitest";
import type { RunEvent, RuntimeEvent } from "./wire.generated";
import {
  RUN_EVENT_METHOD,
  RUNTIME_EVENT_METHOD,
  streamRunEvents,
  streamRuntimeEvents,
} from "./stream";
import type { WireNotificationName, WireNotificationParams } from "./wire.validate.generated";

function fakeClient() {
  const subs = new Map<WireNotificationName, Set<NotificationObserver>>();
  const streamEndHandlers = new Set<StreamEndHandler>();
  let active = 0;
  const client = {
    call: async () => {
      throw new Error("unused");
    },
    close: async () => undefined,
    subscribe(method: WireNotificationName, observer: NotificationObserver) {
      let set = subs.get(method);
      if (!set) {
        set = new Set();
        subs.set(method, set);
      }
      set.add(observer);
      active++;
      return () => {
        set.delete(observer);
        active--;
      };
    },
    onStreamEnd(handler: StreamEndHandler) {
      streamEndHandlers.add(handler);
      active++;
      return () => {
        streamEndHandlers.delete(handler);
        active--;
      };
    },
  } as unknown as RpcClient;

  const emitTo = <M extends WireNotificationName>(
    method: M,
    params: WireNotificationParams[M],
    requestRpcId = "rpc_run",
  ) => {
    for (const observer of subs.get(method) ?? []) {
      observer.next(params, requestRpcId);
    }
  };
  const emit = (params: RunEvent, requestRpcId = "rpc_run") =>
    emitTo(RUN_EVENT_METHOD, params, requestRpcId);
  const emitRuntime = (event: RuntimeEvent, requestRpcId: string) =>
    emitTo(RUNTIME_EVENT_METHOD, { event }, requestRpcId);
  const emitDown = (
    requestRpcId = "rpc_run",
    method: "runs.start" | "runtime.subscribe" = "runs.start",
  ) => {
    for (const handler of streamEndHandlers) {
      handler({ type: "streamEnd", method, requestRpcId });
    }
  };
  return { client, emit, emitRuntime, emitDown, activeCount: () => active };
}

function evt(
  runId: string,
  segmentId: string,
  eventId: string,
  event: RunEvent["event"],
): RunEvent {
  return { runId, segmentId, eventId, timestamp: "2026-06-03T00:00:00Z", event } as RunEvent;
}

// A root-segment segment.started — it lands first on every real run stream.
function rootStarted(): RunEvent {
  return evt("run_root", "seg_root", "evt_start", {
    type: "segment.started",
    run: { id: "run_root", sessionId: "s" } as never,
  });
}

describe("streamRunEvents — tree membership (bound)", () => {
  it("yields tree events and ends on the root-segment segment.finished, no leaked subscriber", async () => {
    const { client, emit, activeCount } = fakeClient();
    const stream = streamRunEvents(client);
    stream.bindRequest("rpc_run");
    stream.bind("run_root", "seg_root");

    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev.event.type);
    })();

    await Promise.resolve();
    // Foreign segment (different segmentId AND runId) — dropped.
    emit(
      evt("run_other", "seg_other", "evt_x", {
        type: "segment.started",
        run: { id: "run_other", sessionId: "s" } as never,
      }),
    );
    emit(
      evt("run_root", "seg_root", "evt_1", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    emit(
      evt("run_root", "seg_root", "evt_2", {
        type: "segment.finished",
        outcome: { type: "completed" },
        metrics: { steps: 0, activeDurationMillis: 0 },
      }),
    );
    await consume;

    expect(collected).toEqual(["item.started", "segment.finished"]); // foreign dropped; finish closes
    expect(activeCount()).toBe(0);
  });

  it("admits a subagent run spawned by an item seen on the tree", async () => {
    const { client, emit } = fakeClient();
    const stream = streamRunEvents(client);
    stream.bindRequest("rpc_run");
    stream.bind("run_root", "seg_root");
    const collected: RunEvent[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev);
    })();

    await Promise.resolve();
    emit(
      evt("run_root", "seg_root", "evt_1", {
        type: "item.started",
        item: { id: "item_tool", type: "toolCall" } as never,
      }),
    );
    // The subagent runs on its OWN segment (seg_child) with its OWN runId.
    emit(
      evt("run_child", "seg_child", "evt_2", {
        type: "segment.started",
        run: { id: "run_child", sessionId: "s", spawnedByItemId: "item_tool" } as never,
      }),
    );
    emit(
      evt("run_child", "seg_child", "evt_3", {
        type: "item.started",
        item: { id: "item_c", type: "agentMessage" } as never,
      }),
    );
    // A subagent's segment.finished (different segmentId) must NOT close the stream.
    emit(
      evt("run_child", "seg_child", "evt_4", {
        type: "segment.finished",
        outcome: { type: "completed" },
        metrics: { steps: 0, activeDurationMillis: 0 },
      }),
    );
    emit(
      evt("run_root", "seg_root", "evt_5", {
        type: "segment.finished",
        outcome: { type: "completed" },
        metrics: { steps: 0, activeDurationMillis: 0 },
      }),
    );
    await consume;

    expect(collected.map((e) => e.runId)).toEqual([
      "run_root",
      "run_child",
      "run_child",
      "run_child",
      "run_root",
    ]);
  });

  it("drops a re-delivered eventId (replay/overlap dedupe, §9.2)", async () => {
    const { client, emit } = fakeClient();
    const stream = streamRunEvents(client);
    stream.bindRequest("rpc_run");
    stream.bind("run_root", "seg_root");
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev.eventId);
    })();

    await Promise.resolve();
    const started = evt("run_root", "seg_root", "evt_1", {
      type: "item.started",
      item: { id: "item_1", type: "agentMessage" } as never,
    });
    emit(started);
    emit(started); // replay overlap re-delivers the same eventId
    emit(
      evt("run_root", "seg_root", "evt_2", {
        type: "segment.finished",
        outcome: { type: "completed" },
        metrics: { steps: 0, activeDurationMillis: 0 },
      }),
    );
    await consume;

    expect(collected).toEqual(["evt_1", "evt_2"]);
  });

  it("unsubscribes on early break", async () => {
    const { client, emit, activeCount } = fakeClient();
    const stream = streamRunEvents(client);
    stream.bindRequest("rpc_run");
    stream.bind("run_root", "seg_root");
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) {
        collected.push(ev.event.type);
        break;
      }
    })();
    await Promise.resolve();
    emit(
      evt("run_root", "seg_root", "evt_1", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    await consume;
    expect(collected).toEqual(["item.started"]);
    expect(activeCount()).toBe(0);
    expect(stream.requestSignal.aborted).toBe(true);
  });

  it("its owning stream-end closes the stream (consumer unblocks)", async () => {
    const { client, emit, emitDown, activeCount } = fakeClient();
    const stream = streamRunEvents(client);
    stream.bindRequest("rpc_run");
    stream.bind("run_root", "seg_root");
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev.event.type);
    })();

    await Promise.resolve();
    emit(rootStarted());
    emit(
      evt("run_root", "seg_root", "evt_1", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    // Transport reports the SSE stream carrying run_root died (no segment.finished
    // ever arrived) — the consumer's for-await must end, not hang forever.
    emitDown();
    await consume;

    expect(collected).toEqual(["segment.started", "item.started"]);
    expect(activeCount()).toBe(0);
  });

  it("remembers a stream-end that arrives before the call result is bound", async () => {
    const { client, emitDown, activeCount } = fakeClient();
    const stream = streamRunEvents(client);
    stream.bindRequest("rpc_run");
    const iterator = stream.events[Symbol.asyncIterator]();
    const next = iterator.next();

    // The HTTP reader can deliver response + immediate EOS before the
    // Promise continuation binds the returned run and segment IDs.
    emitDown();
    stream.bind("run_root", "seg_root");

    await expect(next).resolves.toMatchObject({ done: true });
    expect(activeCount()).toBe(0);
  });

  it("a stream-end for an unrelated request leaves the stream open", async () => {
    const { client, emit, emitDown } = fakeClient();
    const stream = streamRunEvents(client);
    stream.bindRequest("rpc_run");
    stream.bind("run_root", "seg_root");
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev.event.type);
    })();

    await Promise.resolve();
    emitDown("rpc_other");
    emit(
      evt("run_root", "seg_root", "evt_1", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    emit(
      evt("run_root", "seg_root", "evt_2", {
        type: "segment.finished",
        outcome: { type: "completed" },
        metrics: { steps: 0, activeDurationMillis: 0 },
      }),
    );
    await consume;

    expect(collected).toEqual(["item.started", "segment.finished"]);
  });
});

describe("streamRunEvents — deferred bind lifecycle", () => {
  it("subscribes to run events and transport stream-end events on creation", () => {
    const { client, activeCount } = fakeClient();
    streamRunEvents(client);
    expect(activeCount()).toBe(2);
  });

  it("dispose() before bind tears down the subscription (failed runs.start path)", () => {
    // runs.start can reject before bind() — without dispose the unbound
    // subscription would buffer every run event in the app, forever.
    const { client, activeCount } = fakeClient();
    const stream = streamRunEvents(client);
    expect(activeCount()).toBe(2);
    stream.dispose();
    expect(activeCount()).toBe(0);
    expect(stream.requestSignal.aborted).toBe(true);
  });

  it("short-circuits + cleans up on an already-aborted signal", async () => {
    const { client, activeCount } = fakeClient();
    const stream = streamRunEvents(client, AbortSignal.abort());
    const collected: unknown[] = [];
    for await (const ev of stream.events) collected.push(ev);
    expect(collected).toEqual([]);
    expect(activeCount()).toBe(0);
  });

  it("buffers events before bind, then replays through the tree filter", async () => {
    const { client, emit } = fakeClient();
    const { events, bind, bindRequest } = streamRunEvents(client);
    bindRequest("rpc_run");
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of events) collected.push(ev.event.type);
    })();

    await Promise.resolve();
    // Arrive before we know our root segment id — must be buffered.
    emit(rootStarted());
    emit(
      evt("run_root", "seg_root", "evt_2", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    bind("run_root", "seg_root");
    emit(
      evt("run_root", "seg_root", "evt_3", {
        type: "segment.finished",
        outcome: { type: "completed" },
        metrics: { steps: 0, activeDurationMillis: 0 },
      }),
    );
    await consume;

    expect(collected).toEqual(["segment.started", "item.started", "segment.finished"]);
  });
});

describe("streamRuntimeEvents — response-stream ownership", () => {
  it("isolates concurrent subscriptions on the same RpcClient", async () => {
    const { client, emitRuntime, emitDown, activeCount } = fakeClient();
    const sessions = streamRuntimeEvents(client);
    const schedules = streamRuntimeEvents(client);
    sessions.bindRequest("rpc_sessions");
    schedules.bindRequest("rpc_schedules");

    const sessionsIterator = sessions.events[Symbol.asyncIterator]();
    const schedulesIterator = schedules.events[Symbol.asyncIterator]();
    emitRuntime(
      { type: "sessions.changed", sequence: 1, sessionIds: ["session_1"] },
      "rpc_sessions",
    );
    emitRuntime(
      { type: "schedules.changed", sequence: 1, scheduleIds: ["schedule_1"] },
      "rpc_schedules",
    );

    await expect(sessionsIterator.next()).resolves.toMatchObject({
      value: { type: "sessions.changed" },
      done: false,
    });
    await expect(schedulesIterator.next()).resolves.toMatchObject({
      value: { type: "schedules.changed" },
      done: false,
    });

    emitDown("rpc_sessions", "runtime.subscribe");
    await expect(sessionsIterator.next()).resolves.toMatchObject({ done: true });
    expect(activeCount()).toBe(2);
    emitDown("rpc_schedules", "runtime.subscribe");
    await expect(schedulesIterator.next()).resolves.toMatchObject({ done: true });
    expect(activeCount()).toBe(0);
  });

  it("rejects rebinding a stream to a different request", () => {
    const { client } = fakeClient();
    const stream = streamRuntimeEvents(client);
    stream.bindRequest("rpc_1");
    expect(() => stream.bindRequest("rpc_2")).toThrow(/already bound/);
    stream.dispose();
  });
});
