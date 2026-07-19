import { describe, expect, it, vi } from "vitest";
import { createRpcClient, type RpcCallOptions, type RpcClient } from "./client";
import { RpcError, RpcTransportError } from "./errors";
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
  it("reuses a mutation key after an indeterminate failure and rotates after success", async () => {
    const call = vi
      .fn()
      .mockRejectedValueOnce(new RpcTransportError("connection reset"))
      .mockResolvedValueOnce({ sessionId: "ses_1", runId: "run_1" })
      .mockResolvedValueOnce({ sessionId: "ses_2", runId: "run_2" });
    const client = { call } as unknown as RpcClient;
    const methods = createMethods(client);

    await expect(methods.schedules.runNow("schedule_1")).rejects.toBeInstanceOf(RpcTransportError);
    await expect(methods.schedules.runNow("schedule_1")).resolves.toMatchObject({
      runId: "run_1",
    });
    await expect(methods.schedules.runNow("schedule_1")).resolves.toMatchObject({
      runId: "run_2",
    });

    const keys = call.mock.calls.map((args) => args[2]?.idempotencyKey as string);
    expect(keys[0]).toBeTruthy();
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[1]);
  });

  it("retains a mutation key while the original execution is in progress", async () => {
    const call = vi
      .fn()
      .mockRejectedValueOnce(
        new RpcError({
          code: -32021,
          message: "idempotency_in_progress",
          data: { type: "idempotency_in_progress", channel: "rpc", retryable: true },
        }),
      )
      .mockResolvedValueOnce({ id: "session_1" });
    const client = { call } as unknown as RpcClient;
    const methods = createMethods(client);

    await expect(methods.sessions.create({ title: "same", cwd: "/repo" })).rejects.toBeInstanceOf(
      RpcError,
    );
    await methods.sessions.create({ cwd: "/repo", title: "same" });

    const first = call.mock.calls[0]?.[2] as RpcCallOptions | undefined;
    const second = call.mock.calls[1]?.[2] as RpcCallOptions | undefined;
    expect(second?.idempotencyKey).toBe(first?.idempotencyKey);
  });

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

  it("runs.list forwards session filtering and pagination", async () => {
    const t = createMemoryTransport();
    const client = createRpcClient(t);
    const methods = createMethods(client);

    const promise = methods.runs.list({
      sessionId: asSessionId("ses_1"),
      cursor: "next",
      limit: 25,
    });
    const req = await waitForRequest(t, "runs.list");
    expect(req.params).toEqual({ sessionId: "ses_1", cursor: "next", limit: 25 });

    t.inject({ jsonrpc: JSONRPC_VERSION, id: req.id, result: { data: [] } } as RpcMessage);
    await expect(promise).resolves.toEqual({ data: [] });
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
