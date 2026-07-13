import { describe, expect, it } from "vitest";
import { createRpcClient } from "./client";
import { asRunId, asSessionId } from "./ids";
import { createMethods } from "./methods";
import type { RunEvent, StreamEvent } from "./shapes";
import { RUN_EVENT_METHOD } from "./stream";
import { createMemoryTransport } from "./transports/memory";
import { waitForRequest } from "./transports/memory.testkit";
import type { RpcMessage } from "./types";
import { JSONRPC_VERSION } from "./types";

function runEvent(runId: string, segmentId: string, eventId: string, event: StreamEvent): RunEvent {
  return { runId, segmentId, eventId, timestamp: "2026-06-03T00:00:00Z", event } as RunEvent;
}

describe("methods factory", () => {
  it("sessions.list sends sessions.list with optional query and returns a Page", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const promise = methods.sessions.list({ limit: 10 });
    const req = await waitForRequest(t, "sessions.list");
    expect(req.method).toBe("sessions.list");
    expect(req.params).toEqual({ limit: 10 });

    t.inject({ jsonrpc: JSONRPC_VERSION, id: req.id, result: { data: [] } } as RpcMessage);
    await expect(promise).resolves.toEqual({ data: [] });
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

  it("runs.start returns a streaming result that ends on the root segment's segment.finished", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "hi" }],
    });
    const req = await waitForRequest(t, "runs.start");
    expect(req.params).toMatchObject({ sessionId: "ses_1" });

    t.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: { runId: "run_1", segmentId: "seg_1" },
    } as RpcMessage);
    const { result, events } = await startPromise;
    expect(result.runId).toBe("run_1");

    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: RUN_EVENT_METHOD,
      params: runEvent("run_1", "seg_1", "evt_1", {
        type: "item.started",
        item: { id: asRunId("item_1"), type: "agentMessage" } as never,
      }),
    });
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: RUN_EVENT_METHOD,
      params: runEvent("run_1", "seg_1", "evt_2", {
        type: "segment.finished",
        outcome: { type: "completed", result: {} },
      }),
    });

    const collected: RunEvent[] = [];
    for await (const ev of events) collected.push(ev);
    expect(collected.map((e) => e.event.type)).toEqual(["item.started", "segment.finished"]);
    await client.close();
  });

  it("ignores events for foreign segments", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const startPromise = methods.runs.start({
      sessionId: asSessionId("ses_1"),
      input: [{ type: "text", text: "hi" }],
    });
    const req = await waitForRequest(t, "runs.start");
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      id: req.id,
      result: { runId: "run_1", segmentId: "seg_1" },
    } as RpcMessage);
    const { events } = await startPromise;

    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: RUN_EVENT_METHOD,
      params: runEvent("run_OTHER", "seg_OTHER", "evt_x", {
        type: "item.started",
        item: { id: asRunId("item_x"), type: "agentMessage" } as never,
      }),
    });
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: RUN_EVENT_METHOD,
      params: runEvent("run_1", "seg_1", "evt_1", {
        type: "item.completed",
        item: { id: asRunId("item_1"), type: "agentMessage" } as never,
      }),
    });
    t.inject({
      jsonrpc: JSONRPC_VERSION,
      method: RUN_EVENT_METHOD,
      params: runEvent("run_1", "seg_1", "evt_2", {
        type: "segment.finished",
        outcome: { type: "completed", result: {} },
      }),
    });

    const collected: RunEvent[] = [];
    for await (const ev of events) collected.push(ev);
    expect(collected.map((e) => e.event.type)).toEqual(["item.completed", "segment.finished"]);
    await client.close();
  });
});
