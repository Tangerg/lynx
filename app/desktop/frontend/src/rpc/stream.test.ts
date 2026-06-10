// Run-event stream lifecycle (API.md §5 / §10). The headline guarantees:
//   - the stream ends on the ROOT run's `run.finished` (no separate
//     "closed" method in v2);
//   - a single root stream carries the whole run tree (subagent runs are
//     admitted via spawnedByItemId);
//   - the client subscription is torn down on BOTH natural completion and
//     early break (otherwise every finished run leaks a subscriber).

import type { NotificationHandler, RpcClient } from "./client";
import { describe, expect, it } from "vitest";
import type { RunEvent } from "./shapes";
import { RUN_EVENT_METHOD, streamRunEvents, streamRunEventsDeferred } from "./stream";
import { STREAM_DOWN_METHOD } from "./transport";
import { JSONRPC_VERSION } from "./types";

function fakeClient() {
  const subs = new Map<string, Set<NotificationHandler>>();
  let active = 0;
  const client = {
    call: async () => {
      throw new Error("unused");
    },
    notify: async () => undefined,
    close: async () => undefined,
    subscribe(method: string, handler: NotificationHandler) {
      let set = subs.get(method);
      if (!set) {
        set = new Set();
        subs.set(method, set);
      }
      set.add(handler);
      active++;
      return () => {
        set.delete(handler);
        active--;
      };
    },
  } as unknown as RpcClient;

  const emitTo = (method: string, params: unknown) => {
    for (const h of subs.get(method) ?? [])
      h({ jsonrpc: JSONRPC_VERSION, method, params } as Parameters<NotificationHandler>[0]);
  };
  const emit = (params: unknown) => emitTo(RUN_EVENT_METHOD, params);
  const emitDown = (runIds: string[]) => emitTo(STREAM_DOWN_METHOD, { runIds });
  return { client, emit, emitDown, activeCount: () => active };
}

function evt(runId: string, eventId: string, event: RunEvent["event"]): RunEvent {
  return { runId, eventId, timestamp: "2026-06-03T00:00:00Z", event } as RunEvent;
}

describe("streamRunEvents", () => {
  it("subscribes to the run-event + stream-down methods on creation", () => {
    const { client, activeCount } = fakeClient();
    streamRunEvents(client, "run_root");
    expect(activeCount()).toBe(2);
  });

  it("yields tree events and ends on the root run.finished, no leaked subscriber", async () => {
    const { client, emit, activeCount } = fakeClient();
    const stream = streamRunEvents(client, "run_root");

    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev.event.type);
    })();

    await Promise.resolve();
    emit(
      evt("run_other", "evt_x", {
        type: "run.started",
        run: { id: "run_other", sessionId: "s" } as never,
      }),
    );
    emit(
      evt("run_root", "evt_1", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    emit(
      evt("run_root", "evt_2", {
        type: "run.finished",
        outcome: { type: "completed", result: {} },
      }),
    );
    await consume;

    expect(collected).toEqual(["item.started", "run.finished"]); // run_other dropped; finish yielded then closes
    expect(activeCount()).toBe(0);
  });

  it("admits a subagent run spawned by an item seen on the tree", async () => {
    const { client, emit } = fakeClient();
    const stream = streamRunEvents(client, "run_root");
    const collected: RunEvent[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev);
    })();

    await Promise.resolve();
    emit(
      evt("run_root", "evt_1", {
        type: "item.started",
        item: { id: "item_tool", type: "toolCall" } as never,
      }),
    );
    emit(
      evt("run_child", "evt_2", {
        type: "run.started",
        run: { id: "run_child", sessionId: "s", spawnedByItemId: "item_tool" } as never,
      }),
    );
    emit(
      evt("run_child", "evt_3", {
        type: "item.started",
        item: { id: "item_c", type: "agentMessage" } as never,
      }),
    );
    emit(
      evt("run_root", "evt_4", {
        type: "run.finished",
        outcome: { type: "completed", result: {} },
      }),
    );
    await consume;

    expect(collected.map((e) => e.runId)).toEqual([
      "run_root",
      "run_child",
      "run_child",
      "run_root",
    ]);
  });

  it("unsubscribes on early break", async () => {
    const { client, emit, activeCount } = fakeClient();
    const stream = streamRunEvents(client, "run_root");
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) {
        collected.push(ev.event.type);
        break;
      }
    })();
    await Promise.resolve();
    emit(
      evt("run_root", "evt_1", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    await consume;
    expect(collected).toEqual(["item.started"]);
    expect(activeCount()).toBe(0);
  });

  it("short-circuits + cleans up on an already-aborted signal", async () => {
    const { client, activeCount } = fakeClient();
    const stream = streamRunEvents(client, "run_root", AbortSignal.abort());
    const collected: unknown[] = [];
    for await (const ev of stream.events) collected.push(ev);
    expect(collected).toEqual([]);
    expect(activeCount()).toBe(0);
  });

  it("dispose() tears down the subscriptions without anyone iterating", () => {
    // The owning call (runs.subscribe) can reject before any consumer touches
    // `events` — dispose is the only cleanup path then.
    const { client, activeCount } = fakeClient();
    const stream = streamRunEvents(client, "run_root");
    expect(activeCount()).toBe(2);
    stream.dispose();
    expect(activeCount()).toBe(0);
  });

  it("a stream-down naming a tree run closes the stream (consumer unblocks)", async () => {
    const { client, emit, emitDown, activeCount } = fakeClient();
    const stream = streamRunEvents(client, "run_root");
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev.event.type);
    })();

    await Promise.resolve();
    emit(
      evt("run_root", "evt_1", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    // Transport reports the SSE stream carrying run_root died (no run.finished
    // ever arrived) — the consumer's for-await must end, not hang forever.
    emitDown(["run_root"]);
    await consume;

    expect(collected).toEqual(["item.started"]);
    expect(activeCount()).toBe(0);
  });

  it("a stream-down for an unrelated run leaves the stream open", async () => {
    const { client, emit, emitDown } = fakeClient();
    const stream = streamRunEvents(client, "run_root");
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev.event.type);
    })();

    await Promise.resolve();
    emitDown(["run_other"]); // some other run's stream died — not ours
    emit(
      evt("run_root", "evt_1", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    emit(
      evt("run_root", "evt_2", {
        type: "run.finished",
        outcome: { type: "completed", result: {} },
      }),
    );
    await consume;

    expect(collected).toEqual(["item.started", "run.finished"]);
  });

  it("drops a re-delivered eventId (replay/overlap dedupe, §9.2)", async () => {
    const { client, emit } = fakeClient();
    const stream = streamRunEvents(client, "run_root");
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev.eventId);
    })();

    await Promise.resolve();
    const started = evt("run_root", "evt_1", {
      type: "item.started",
      item: { id: "item_1", type: "agentMessage" } as never,
    });
    emit(started);
    emit(started); // replay overlap re-delivers the same eventId
    emit(
      evt("run_root", "evt_2", {
        type: "run.finished",
        outcome: { type: "completed", result: {} },
      }),
    );
    await consume;

    expect(collected).toEqual(["evt_1", "evt_2"]);
  });
});

describe("streamRunEventsDeferred", () => {
  it("dispose() before bind tears down the subscription (failed runs.start path)", () => {
    // runs.start can reject before bind() — without dispose the unbound
    // subscription would buffer every run event in the app, forever.
    const { client, activeCount } = fakeClient();
    const stream = streamRunEventsDeferred(client);
    expect(activeCount()).toBe(2);
    stream.dispose();
    expect(activeCount()).toBe(0);
  });

  it("buffers events before bind, then replays through the tree filter", async () => {
    const { client, emit } = fakeClient();
    const { events, bind } = streamRunEventsDeferred(client);
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of events) collected.push(ev.event.type);
    })();

    await Promise.resolve();
    // Arrive before we know our runId — must be buffered.
    emit(
      evt("run_root", "evt_1", {
        type: "run.started",
        run: { id: "run_root", sessionId: "s" } as never,
      }),
    );
    emit(
      evt("run_root", "evt_2", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    bind("run_root");
    emit(
      evt("run_root", "evt_3", {
        type: "run.finished",
        outcome: { type: "completed", result: {} },
      }),
    );
    await consume;

    expect(collected).toEqual(["run.started", "item.started", "run.finished"]);
  });
});
