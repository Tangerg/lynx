import { describe, expect, it, vi } from "vitest";
import { createRpcClient } from "./client";
import { RpcError, RpcProtocolError, RpcTransportError } from "./errors";
import { createMemoryTransport } from "./transports/memory";
import { waitForRequest } from "./transports/memory.testkit";
import type { Transport } from "./transport";
import type { RpcMessage } from "./types";
import { JSONRPC_VERSION, RPC_METHOD_NOT_FOUND } from "./types";
import session from "./samples/session.json";

const SOME_BUSINESS_CODE = -32002;

describe("RpcClient", () => {
  it("sends a registered Request and resolves a validated result", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    const promise = client.call("sessions.get", { sessionId: "ses_01" });
    const request = await waitForRequest(transport, "sessions.get");
    expect(request.jsonrpc).toBe(JSONRPC_VERSION);

    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: request.id,
      result: session,
    } as RpcMessage);

    await expect(promise).resolves.toEqual(session);
    await client.close();
  });

  it("rejects a malformed method result at the generated contract boundary", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    const promise = client.call("sessions.get", { sessionId: "ses_01" });
    const request = await waitForRequest(transport, "sessions.get");
    const { revision: _revision, ...malformed } = session;
    transport.inject({ jsonrpc: JSONRPC_VERSION, id: request.id, result: malformed } as RpcMessage);

    await expect(promise).rejects.toMatchObject({
      name: "RpcProtocolError",
      violations: [{ path: "sessions.get.result.revision", detail: "is required" }],
    } satisfies Partial<RpcProtocolError>);
    await client.close();
  });

  it("rejects a response carried by another request without settling that request", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    const sessionsPromise = client.call("sessions.get", { sessionId: "ses_01" });
    const runsPromise = client.call("runs.list", {});
    const sessionsRequest = await waitForRequest(transport, "sessions.get");
    const runsRequest = await waitForRequest(transport, "runs.list");

    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        id: runsRequest.id,
        result: { data: [] },
      } as RpcMessage,
      { requestId: "req_mismatch_01" },
      sessionsRequest.id,
    );
    await expect(sessionsPromise).rejects.toMatchObject({
      name: "RpcProtocolError",
      requestId: "req_mismatch_01",
      violations: [
        {
          path: "sessions.get.response.id",
          detail: `must match request id ${sessionsRequest.id}`,
        },
      ],
    } satisfies Partial<RpcProtocolError>);

    transport.inject({
      jsonrpc: JSONRPC_VERSION,
      id: runsRequest.id,
      result: { data: [] },
    } as RpcMessage);
    await expect(runsPromise).resolves.toEqual({ data: [] });
    await client.close();
  });

  it("validates an ack result and exposes it as void", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    const promise = client.call("sessions.delete", { sessionId: "ses_01" });
    const request = await waitForRequest(transport, "sessions.delete");
    transport.inject({ jsonrpc: JSONRPC_VERSION, id: request.id, result: {} });

    await expect(promise).resolves.toBeUndefined();
    await client.close();
  });

  it("rejects with RpcError on a valid error response", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);

    const promise = client.call("sessions.get", { sessionId: "missing" });
    const request = await waitForRequest(transport, "sessions.get");
    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        id: request.id,
        error: {
          code: SOME_BUSINESS_CODE,
          message: "not found",
          data: { type: "session_not_found", recoveryAction: "refetch" },
        },
      },
      { requestId: "req_business_01" },
    );

    await expect(promise).rejects.toMatchObject({
      name: "RpcError",
      code: SOME_BUSINESS_CODE,
      requestId: "req_business_01",
    } satisfies Partial<RpcError>);
    await client.close();
  });

  it("dispatches only validated published notifications", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    const next = vi.fn();
    const error = vi.fn();
    const unsubscribe = client.subscribe("notifications.runtime.event", { next, error });

    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: "notifications.runtime.event",
        params: { event: { type: "skills.changed", sequence: 1 } },
      },
      undefined,
      "rpc_runtime",
    );
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(next).toHaveBeenCalledOnce();
    expect(error).not.toHaveBeenCalled();
    unsubscribe();
    await client.close();
  });

  it("terminates subscribers when notification params violate the contract", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    const next = vi.fn();
    const error = vi.fn();
    client.subscribe("notifications.runtime.event", { next, error });

    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: "notifications.runtime.event",
        params: { event: { type: "skills.changed", sequence: 0 } },
      },
      undefined,
      "rpc_runtime",
    );
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(next).not.toHaveBeenCalled();
    expect(error).toHaveBeenCalledWith(expect.any(RpcProtocolError), "rpc_runtime");
    await client.close();
  });

  it("drops a malformed ephemeral run event without terminating its stream", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    const next = vi.fn();
    const error = vi.fn();
    client.subscribe("notifications.run.event", { next, error });

    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: "notifications.run.event",
        params: {
          runId: "run_01",
          segmentId: "seg_01",
          eventId: "evt_01",
          timestamp: "2026-08-02T00:00:00Z",
          event: { type: "item.delta" },
        },
      },
      undefined,
      "rpc_run",
    );
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(next).not.toHaveBeenCalled();
    expect(error).not.toHaveBeenCalled();
    expect(warn).toHaveBeenCalledWith(expect.stringContaining("invalid ephemeral notification"));
    warn.mockRestore();
    await client.close();
  });

  it("terminates a run stream when an authoritative event is malformed", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    const next = vi.fn();
    const error = vi.fn();
    client.subscribe("notifications.run.event", { next, error });

    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: "notifications.run.event",
        params: {
          runId: "run_01",
          segmentId: "seg_01",
          eventId: "evt_01",
          timestamp: "2026-08-02T00:00:00Z",
          event: { type: "segment.finished" },
        },
      },
      undefined,
      "rpc_run",
    );
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(next).not.toHaveBeenCalled();
    expect(error).toHaveBeenCalledWith(expect.any(RpcProtocolError), "rpc_run");
    await client.close();
  });

  it("close rejects pending calls and prevents further use", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    const promise = client.call("sessions.get", { sessionId: "ses_01" });
    await waitForRequest(transport, "sessions.get");

    await client.close();
    await expect(promise).rejects.toBeInstanceOf(RpcTransportError);
    await expect(client.call("sessions.get", { sessionId: "ses_01" })).rejects.toBeInstanceOf(
      RpcTransportError,
    );
  });

  it("keeps concurrent close callers joined to the same transport teardown", async () => {
    const inner = createMemoryTransport();
    let release!: () => void;
    const held = new Promise<void>((resolve) => {
      release = resolve;
    });
    const close = vi.fn(async () => {
      await held;
      await inner.close();
    });
    const client = createRpcClient({ ...inner, close });

    const first = client.close();
    let secondSettled = false;
    const second = client.close().then(() => {
      secondSettled = true;
    });
    await Promise.resolve();

    expect(close).toHaveBeenCalledOnce();
    expect(secondSettled).toBe(false);

    release();
    await Promise.all([first, second]);
  });

  it("still tears down the transport after its receive stream ended", async () => {
    let endReceive!: () => void;
    const receiveEnded = new Promise<void>((resolve) => {
      endReceive = resolve;
    });
    const close = vi.fn(async () => undefined);
    const transport: Transport = {
      send: vi.fn(async () => undefined),
      recv: () =>
        (async function* () {
          await receiveEnded;
          yield* [];
        })(),
      close,
    };
    const client = createRpcClient(transport);

    endReceive();
    await vi.waitFor(() =>
      expect(client.call("sessions.get", { sessionId: "ses_01" })).rejects.toBeInstanceOf(
        RpcTransportError,
      ),
    );
    await client.close();

    expect(close).toHaveBeenCalledOnce();
  });

  it("rejects in-flight calls when the transport stream ends cleanly", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    const promise = client.call("sessions.get", { sessionId: "ses_01" });
    await waitForRequest(transport, "sessions.get");

    await transport.close();
    await expect(promise).rejects.toBeInstanceOf(RpcTransportError);
    await expect(client.call("sessions.get", { sessionId: "ses_01" })).rejects.toBeInstanceOf(
      RpcTransportError,
    );
  });

  it("AbortSignal cancels an in-flight call without a second protocol message", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    const controller = new AbortController();
    const promise = client.call(
      "runs.start",
      { sessionId: "ses_01", input: [{ type: "text", text: "hello" }] },
      { signal: controller.signal },
    );
    await waitForRequest(transport, "runs.start");
    controller.abort();

    await expect(promise).rejects.toBeInstanceOf(RpcTransportError);
    expect(transport.outbox()).toHaveLength(1);
    await client.close();
  });

  it("ignores unknown server notifications without poisoning correlation", async () => {
    const transport = createMemoryTransport();
    const client = createRpcClient(transport);
    transport.inject(
      {
        jsonrpc: JSONRPC_VERSION,
        method: "notifications.unknown",
        params: {},
      },
      undefined,
      "rpc_unknown",
    );
    await Promise.resolve();

    const promise = client.call("sessions.get", { sessionId: "ses_01" });
    const request = await waitForRequest(transport, "sessions.get");
    transport.inject({ jsonrpc: JSONRPC_VERSION, id: request.id, result: session } as RpcMessage);
    await expect(promise).resolves.toEqual(session);
    await client.close();
    expect(RPC_METHOD_NOT_FOUND).toBeLessThan(0);
  });
});
