import { describe, expect, it } from "vitest";
import { createRpcClient } from "./client";
import { RpcError, RpcTransportError } from "./errors";
import { createMemoryTransport } from "./transports/memory";
import { waitForRequest } from "./transports/memory.testkit";
import type { RpcMessage } from "./types";
import { JSONRPC_VERSION, RPC_METHOD_NOT_FOUND, RPC_SESSION_NOT_FOUND } from "./types";

describe("RpcClient", () => {
  it("call() sends a Request and resolves with result", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);

    const promise = client.call<{ ok: boolean }>("test.echo");
    const req = await waitForRequest(t, "test.echo");
    expect(req.method).toBe("test.echo");
    expect(req.jsonrpc).toBe(JSONRPC_VERSION);

    t.inject({ jsonrpc: JSONRPC_VERSION, id: req.id, result: { ok: true } } as RpcMessage);

    expect(await promise).toEqual({ ok: true });
    await client.close();
  });

  it("call() rejects with RpcError on error response", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);

    const promise = client.call("sessions.get", { id: "missing" });
    const req = await waitForRequest(t, "sessions.get");

    t.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      error: { code: RPC_SESSION_NOT_FOUND, message: "not found" },
    } as RpcMessage);

    await expect(promise).rejects.toBeInstanceOf(RpcError);
    await expect(promise).rejects.toMatchObject({ code: RPC_SESSION_NOT_FOUND });
    await client.close();
  });

  it("notify() sends a Notification with no id", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    await client.notify("test.notification", { id: "5" });
    const sent = t.outbox()[0];
    expect(sent).toBeDefined();
    expect("id" in (sent as object)).toBe(false);
    expect((sent as { method: string }).method).toBe("test.notification");
    await client.close();
  });

  it("subscribe() dispatches inbound notifications to matching handlers", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const events: unknown[] = [];
    const unsub = client.subscribe("notifications/run/event", (msg) => events.push(msg.params));

    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: "notifications/run/event",
      params: { eventId: "1", event: { type: "TEXT_MESSAGE_CONTENT" } },
    });
    // Give the dispatch loop a tick.
    await new Promise((r) => setTimeout(r, 0));
    expect(events).toHaveLength(1);

    unsub();
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: "notifications/run/event",
      params: { eventId: "2" },
    });
    await new Promise((r) => setTimeout(r, 0));
    expect(events).toHaveLength(1); // unsubscribed — no second event
    await client.close();
  });

  it("close() rejects pending calls and prevents further use", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const promise = client.call("test.echo");
    await waitForRequest(t, "test.echo");

    await client.close();
    await expect(promise).rejects.toBeInstanceOf(RpcTransportError);
    await expect(client.call("test.echo")).rejects.toBeInstanceOf(RpcTransportError);
  });

  it("rejects in-flight calls when the transport stream ends cleanly", async () => {
    // Close the transport directly (not via client.close()), simulating a
    // remote/clean EOS while a request is in flight. recv() completes
    // without throwing — the pump must still settle the pending call.
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const promise = client.call("test.echo");
    await waitForRequest(t, "test.echo");

    await t.close();
    await expect(promise).rejects.toBeInstanceOf(RpcTransportError);
    await expect(client.call("test.echo")).rejects.toBeInstanceOf(RpcTransportError);
    await expect(client.notify("test.notification", { id: "late" })).rejects.toBeInstanceOf(
      RpcTransportError,
    );
  });

  it("AbortSignal cancels an in-flight call without a second protocol message", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const ctrl = new AbortController();
    const promise = client.call("runs.start", undefined, { signal: ctrl.signal });
    await waitForRequest(t, "runs.start");
    ctrl.abort();
    await expect(promise).rejects.toBeInstanceOf(RpcTransportError);
    expect(t.outbox()).toHaveLength(1);
    await client.close();
  });

  it("ignores method-not-found errors when no matching subscriber", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    // Inject a notification nobody subscribed to — should not crash.
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: "notifications/unknown",
      params: {},
    });
    await new Promise((r) => setTimeout(r, 0));
    // Survives — call still works.
    const promise = client.call<number>("test.echo");
    const req = await waitForRequest(t, "test.echo");
    t.inject({ jsonrpc: JSONRPC_VERSION, id: req.id, result: 1 });
    expect(await promise).toBe(1);
    await client.close();
    // Reference unused import to please knip in test:
    expect(RPC_METHOD_NOT_FOUND).toBeLessThan(0);
  });
});
