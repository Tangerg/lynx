// Run-event stream lifecycle (API.md §5 / §10). The headline guarantees:
//   - the stream ends on the ROOT SEGMENT's `segment.finished` (no separate
//     "closed" method in v2);
//   - a single root-segment stream carries the whole run tree (subagent runs
//     are admitted via spawnedByItemId, keyed on their distinct runIds);
//   - the root segment id is bound from the call response, AFTER the eager
//     subscription (so head events under streamable HTTP aren't dropped);
//   - the client subscription is torn down on BOTH natural completion and
//     early break (otherwise every finished run leaks a subscriber).

import type { NotificationHandler, RpcClient } from "./client";
import { describe, expect, it } from "vitest";
import type { RunEvent } from "./shapes";
import { RUN_EVENT_METHOD, streamRunEvents } from "./stream";
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

function evt(
  runId: string,
  segmentId: string,
  eventId: string,
  event: RunEvent["event"],
): RunEvent {
  return { runId, segmentId, eventId, timestamp: "2026-06-03T00:00:00Z", event } as RunEvent;
}

// A root-segment segment.started — its `run.id` is the root runId (learned by the
// tree for STREAM_DOWN matching); it lands FIRST on every real stream.
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
    stream.bind("seg_root");

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
        outcome: { type: "completed", result: {} },
      }),
    );
    await consume;

    expect(collected).toEqual(["item.started", "segment.finished"]); // foreign dropped; finish closes
    expect(activeCount()).toBe(0);
  });

  it("admits a subagent run spawned by an item seen on the tree", async () => {
    const { client, emit } = fakeClient();
    const stream = streamRunEvents(client);
    stream.bind("seg_root");
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
        outcome: { type: "completed", result: {} },
      }),
    );
    emit(
      evt("run_root", "seg_root", "evt_5", {
        type: "segment.finished",
        outcome: { type: "completed", result: {} },
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
    stream.bind("seg_root");
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
        outcome: { type: "completed", result: {} },
      }),
    );
    await consume;

    expect(collected).toEqual(["evt_1", "evt_2"]);
  });

  it("unsubscribes on early break", async () => {
    const { client, emit, activeCount } = fakeClient();
    const stream = streamRunEvents(client);
    stream.bind("seg_root");
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
  });

  it("a stream-down naming a tree run closes the stream (consumer unblocks)", async () => {
    const { client, emit, emitDown, activeCount } = fakeClient();
    const stream = streamRunEvents(client);
    stream.bind("seg_root");
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev.event.type);
    })();

    await Promise.resolve();
    emit(rootStarted()); // tree learns the root runId (for STREAM_DOWN matching)
    emit(
      evt("run_root", "seg_root", "evt_1", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    // Transport reports the SSE stream carrying run_root died (no segment.finished
    // ever arrived) — the consumer's for-await must end, not hang forever.
    emitDown(["run_root"]);
    await consume;

    expect(collected).toEqual(["segment.started", "item.started"]);
    expect(activeCount()).toBe(0);
  });

  it("a stream-down for an unrelated run leaves the stream open", async () => {
    const { client, emit, emitDown } = fakeClient();
    const stream = streamRunEvents(client);
    stream.bind("seg_root");
    const collected: string[] = [];
    const consume = (async () => {
      for await (const ev of stream.events) collected.push(ev.event.type);
    })();

    await Promise.resolve();
    emitDown(["run_other"]); // some other run's stream died — not ours
    emit(
      evt("run_root", "seg_root", "evt_1", {
        type: "item.started",
        item: { id: "item_1", type: "agentMessage" } as never,
      }),
    );
    emit(
      evt("run_root", "seg_root", "evt_2", {
        type: "segment.finished",
        outcome: { type: "completed", result: {} },
      }),
    );
    await consume;

    expect(collected).toEqual(["item.started", "segment.finished"]);
  });
});

describe("streamRunEvents — deferred bind lifecycle", () => {
  it("subscribes to the run-event + stream-down methods on creation", () => {
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
    const { events, bind } = streamRunEvents(client);
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
    bind("seg_root");
    emit(
      evt("run_root", "seg_root", "evt_3", {
        type: "segment.finished",
        outcome: { type: "completed", result: {} },
      }),
    );
    await consume;

    expect(collected).toEqual(["segment.started", "item.started", "segment.finished"]);
  });
});
