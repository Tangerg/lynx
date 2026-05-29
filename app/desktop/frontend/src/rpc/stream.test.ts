// makeFilteredStream lifecycle — the headline guarantee is that the
// client subscriptions a stream opens (notificationMethod + closedMethod)
// are torn down when the stream ENDS NATURALLY, not just on early break.
// `for await` only calls iterator.return() on break/throw; a stream that
// closes itself (closedMethod / isTerminal / abort) resolves next() with
// done=true and the loop exits without return() — so cleanup has to ride
// on the done result too, or every finished run leaks two subscribers.

import type { NotificationHandler, RpcClient } from "./client";
import { describe, expect, it } from "vitest";
import { makeFilteredStream } from "./stream";
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

  const emit = (method: string, params: unknown) => {
    for (const h of subs.get(method) ?? [])
      h({ jsonrpc: JSONRPC_VERSION, method, params } as Parameters<NotificationHandler>[0]);
  };
  return { client, emit, activeCount: () => active };
}

function runEventStream(client: RpcClient, signal?: AbortSignal) {
  return makeFilteredStream<string, { runId: string }>(client, {
    idField: "runId",
    idValue: "r1",
    notificationMethod: "notifications/run/event",
    parseParams: (raw) => raw as { runId: string },
    extract: () => "evt",
    closedMethod: {
      method: "notifications/run/closed",
      parseParams: (raw) => raw as Record<string, unknown>,
    },
    signal,
  });
}

describe("makeFilteredStream", () => {
  it("subscribes to both the event + closed methods on creation", () => {
    const { client, activeCount } = fakeClient();
    runEventStream(client);
    expect(activeCount()).toBe(2);
  });

  it("unsubscribes when the stream completes naturally (closedMethod)", async () => {
    const { client, emit, activeCount } = fakeClient();
    const stream = runEventStream(client);
    expect(activeCount()).toBe(2);

    const collected: string[] = [];
    const consume = (async () => {
      for await (const v of stream) collected.push(v);
    })();

    await Promise.resolve();
    emit("notifications/run/event", { runId: "r1" });
    await Promise.resolve();
    emit("notifications/run/closed", { runId: "r1" });
    await consume;

    expect(collected).toEqual(["evt"]);
    expect(activeCount()).toBe(0); // no leaked subscribers
  });

  it("unsubscribes on early break (return path)", async () => {
    const { client, emit, activeCount } = fakeClient();
    const stream = runEventStream(client);

    const collected: string[] = [];
    const consume = (async () => {
      for await (const v of stream) {
        collected.push(v);
        break; // early exit → iterator.return()
      }
    })();

    await Promise.resolve();
    emit("notifications/run/event", { runId: "r1" });
    await consume;

    expect(collected).toEqual(["evt"]);
    expect(activeCount()).toBe(0);
  });

  it("unsubscribes when an already-aborted signal short-circuits the stream", async () => {
    const { client, activeCount } = fakeClient();
    const stream = runEventStream(client, AbortSignal.abort());

    const collected: string[] = [];
    for await (const v of stream) collected.push(v);

    expect(collected).toEqual([]);
    expect(activeCount()).toBe(0);
  });
});
