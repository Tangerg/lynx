import { describe, expect, it } from "vitest";
import { createRpcClient } from "./client";
import { createMethods } from "./methods";
import { createMemoryTransport } from "./transports/memory";
import type { RpcMessage, RpcRequest } from "./types";
import { JSONRPC_VERSION } from "./types";

function takeRequest(t: ReturnType<typeof createMemoryTransport>): Promise<RpcRequest> {
  return new Promise((resolve) => {
    const tick = () => {
      const last = t.outbox().at(-1);
      if (last && "id" in last && "method" in last) resolve(last as RpcRequest);
      else setTimeout(tick, 0);
    };
    tick();
  });
}

describe("methods factory", () => {
  it("sessions.list sends sessions.list method with optional query", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const promise = methods.sessions.list({ limit: 10 });
    const req = await takeRequest(t);
    expect(req.method).toBe("sessions.list");
    expect(req.params).toEqual({ limit: 10 });

    t.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: { items: [], hasMore: false },
    } as RpcMessage);
    await expect(promise).resolves.toEqual({ items: [], hasMore: false });
    await client.close();
  });

  it("runtime.shutdown is a notification (no response wait)", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);
    await methods.runtime.shutdown({ reason: "test" });
    const sent = t.outbox()[0] as { method: string; params: { reason: string } };
    expect(sent.method).toBe("runtime.shutdown");
    expect("id" in sent).toBe(false);
    await client.close();
  });

  it("runs.start returns streaming result + AG-UI events iterator", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const startPromise = methods.runs.start({
      sessionId: "s1",
      messages: [],
    });
    const req = await takeRequest(t);
    expect(req.method).toBe("runs.start");

    t.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: { runId: "r1", streamHandle: "h1" },
    } as RpcMessage);

    const { result, events } = await startPromise;
    expect(result.runId).toBe("r1");
    expect(result.streamHandle).toBe("h1");

    // Push two events on the right stream, then close.
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: "notifications/run/event",
      params: {
        streamHandle: "h1",
        eventId: "1",
        event: { type: "TEXT_MESSAGE_START", messageId: "m1" },
      },
    });
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: "notifications/run/event",
      params: {
        streamHandle: "h1",
        eventId: "2",
        event: { type: "TEXT_MESSAGE_END", messageId: "m1" },
      },
    });
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: "notifications/run/closed",
      params: { streamHandle: "h1" },
    });

    const collected: unknown[] = [];
    for await (const ev of events) collected.push(ev);
    expect(collected).toHaveLength(2);

    await client.close();
  });

  it("filters events by streamHandle (ignores other runs)", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const startPromise = methods.runs.start({ sessionId: "s1", messages: [] });
    const req = await takeRequest(t);
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: { runId: "r1", streamHandle: "h1" },
    } as RpcMessage);
    const { events } = await startPromise;

    // Foreign stream — must be ignored.
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: "notifications/run/event",
      params: { streamHandle: "OTHER", eventId: "x", event: { type: "X" } },
    });
    // Our stream + close.
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: "notifications/run/event",
      params: { streamHandle: "h1", eventId: "1", event: { type: "MINE" } },
    });
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: "notifications/run/closed",
      params: { streamHandle: "h1" },
    });

    const collected: Array<{ type?: string }> = [];
    for await (const ev of events) collected.push(ev as { type?: string });
    expect(collected).toHaveLength(1);
    expect(collected[0].type).toBe("MINE");
    await client.close();
  });
});
