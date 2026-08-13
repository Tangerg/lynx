import type { TransportRequest } from "../transport";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RpcConnectionError, RpcTransportError } from "../errors";
import type { WireMethodName } from "../wire.methods.generated";
import { createHttpTransport } from "./http";

afterEach(() => vi.restoreAllMocks());

// A 200 text/event-stream POST response whose body emits the given chunks.
function sseResponse(chunks: string[], requestId?: string): Response {
  const enc = new TextEncoder();
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(enc.encode(c));
      controller.close();
    },
  });
  return new Response(body, {
    status: 200,
    headers: {
      "Content-Type": "text/event-stream",
      ...(requestId ? { "Request-Id": requestId } : {}),
    },
  });
}

// A stream that emits one chunk, then errors with an AbortError on next read —
// models the fetch being aborted (stop / switch session / unmount).
function abortingSseResponse(firstChunk: string): Response {
  const enc = new TextEncoder();
  let sent = false;
  const body = new ReadableStream<Uint8Array>({
    pull(controller) {
      if (!sent) {
        sent = true;
        controller.enqueue(enc.encode(firstChunk));
      } else {
        controller.error(Object.assign(new Error("aborted"), { name: "AbortError" }));
      }
    },
  });
  return new Response(body, { status: 200, headers: { "Content-Type": "text/event-stream" } });
}

function jsonResponse(obj: unknown, requestId?: string): Response {
  return new Response(JSON.stringify(obj), {
    status: 200,
    headers: {
      "Content-Type": "application/json",
      ...(requestId ? { "Request-Id": requestId } : {}),
    },
  });
}

// One SSE frame. `id` omitted ⇒ a response/ack frame (no SSE id); set ⇒ an
// event frame carrying its eventId.
const frame = (obj: unknown, id?: string): string =>
  `${id ? `id: ${id}\n` : ""}data: ${JSON.stringify(obj)}\n\n`;

const req = (id: string, method: WireMethodName): TransportRequest => ({
  jsonrpc: "2.0",
  id,
  method,
  params: {},
});

function deferred(): { promise: Promise<void>; resolve: () => void } {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("HTTPTransport — streamable HTTP", () => {
  it("streaming method: POST response stream yields the call response then its events", async () => {
    const responseFrame = frame({ jsonrpc: "2.0", id: "1", result: { runId: "run_01" } }); // no SSE id
    const started = frame(
      {
        jsonrpc: "2.0",
        method: "notifications.run.event",
        params: { event: { type: "segment.started" } },
      },
      "evt_0001",
    );
    const finished = frame(
      {
        jsonrpc: "2.0",
        method: "notifications.run.event",
        params: { event: { type: "segment.finished" } },
      },
      "evt_0002",
    );
    const wire = responseFrame + started + finished;
    const cut = Math.floor(wire.length / 2); // split mid-stream → parser must buffer across chunks

    const fetchStub = (async () =>
      sseResponse(
        [wire.slice(0, cut), wire.slice(cut)],
        "req_stream_01",
      )) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("1", "runs.start"));
    const r0 = await it.next();
    const r1 = await it.next();
    const r2 = await it.next();
    await transport.close();

    expect(r0.value).toMatchObject({
      type: "message",
      message: { id: "1", result: { runId: "run_01" } },
      requestRpcId: "1",
      metadata: { requestId: "req_stream_01" },
    });
    expect(r1.value).toMatchObject({
      type: "message",
      message: { params: { event: { type: "segment.started" } } },
      requestRpcId: "1",
    });
    expect(r2.value).toMatchObject({
      type: "message",
      message: { params: { event: { type: "segment.finished" } } },
      requestRpcId: "1",
    });
  });

  it("non-streaming method: POST returns a single application/json message", async () => {
    const fetchStub = vi.fn(async () =>
      jsonResponse(
        {
          jsonrpc: "2.0",
          id: "2",
          result: { id: "ses_1" },
        },
        "req_unary_01",
      ),
    );
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("2", "sessions.get"));
    const r = await it.next();
    await transport.close();

    expect(r.value).toMatchObject({
      type: "message",
      message: { id: "2", result: { id: "ses_1" } },
      requestRpcId: "2",
      metadata: { requestId: "req_unary_01" },
    });
    expect(fetchStub).toHaveBeenCalledWith(
      "http://x/v2/rpc",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("rejects an event stream returned for a non-streaming method", async () => {
    const fetchStub = (async () => sseResponse([])) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });

    await expect(transport.send(req("2", "sessions.get"))).rejects.toThrow(
      "non-streaming RPC method sessions.get returned an event stream",
    );
    await transport.close();
  });

  it("sends the logical mutation identity and Runtime store as transport metadata", async () => {
    const fetchStub = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      jsonResponse({ jsonrpc: "2.0", id: "2", result: { id: "ses_1" } }),
    );
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("2", "sessions.create"), undefined, {
      idempotencyKey: "operation-key-1",
      idempotencyNamespace: "idp_store_a",
    });
    await it.next();
    await transport.close();

    const headers = fetchStub.mock.calls[0]?.[1]?.headers as Record<string, string>;
    expect(headers["Idempotency-Key"]).toBe("operation-key-1");
    expect(headers["Idempotency-Namespace"]).toBe("idp_store_a");
  });

  it("rejects a no-content response for a call", async () => {
    const fetchStub = (async () => new Response(null, { status: 204 })) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });

    await expect(transport.send(req("2", "sessions.get"))).rejects.toThrow(
      "RPC call ended without a response",
    );
    await transport.close();
  });

  it("rejects a response correlated to another request", async () => {
    const fetchStub = (async () =>
      jsonResponse({ jsonrpc: "2.0", id: "other", result: {} })) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });

    await expect(transport.send(req("2", "sessions.get"))).rejects.toThrow(
      "does not match the outbound request",
    );
    await transport.close();
  });

  it("close aborts an in-flight request owned by the transport", async () => {
    let requestSignal: AbortSignal | undefined;
    const fetchStub = ((_url: string, init?: RequestInit) => {
      requestSignal = init?.signal ?? undefined;
      return new Promise<Response>((_resolve, reject) => {
        requestSignal?.addEventListener(
          "abort",
          () => reject(new DOMException("aborted", "AbortError")),
          { once: true },
        );
      });
    }) as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const sending = transport.send(req("2", "sessions.get"));
    await Promise.resolve();

    await transport.close();
    await expect(sending).rejects.toThrow("fetch failed: aborted");
    expect(requestSignal?.aborted).toBe(true);
  });

  it("does not report closed until an in-flight request releases after abort", async () => {
    const aborted = deferred();
    const release = deferred();
    const fetchStub = ((_url: string, init?: RequestInit) =>
      new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener(
          "abort",
          () => {
            aborted.resolve();
            void release.promise.then(() => reject(new DOMException("aborted", "AbortError")));
          },
          { once: true },
        );
      })) as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const sending = transport.send(req("2", "sessions.get")).catch((error: unknown) => error);
    let closed = false;
    const closing = transport.close().then(() => {
      closed = true;
    });

    await aborted.promise;
    await Promise.resolve();
    expect(closed).toBe(false);

    release.resolve();
    await closing;
    await expect(sending).resolves.toBeInstanceOf(RpcConnectionError);
  });

  it("does not report closed until an active stream reader releases", async () => {
    const cancelStarted = deferred();
    const release = deferred();
    const pullRelease = deferred();
    const body = new ReadableStream<Uint8Array>({
      pull: () => pullRelease.promise,
      async cancel() {
        cancelStarted.resolve();
        await release.promise;
      },
    });
    const fetchStub = (async () =>
      new Response(body, {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      })) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    await transport.send(req("1", "runs.start"));
    let closed = false;
    const closing = transport.close().then(() => {
      closed = true;
    });

    await cancelStarted.promise;
    await Promise.resolve();
    expect(closed).toBe(false);

    release.resolve();
    await closing;
    pullRelease.resolve();
    await pullRelease.promise;
  });

  it("non-2xx surfaces structured transport diagnostics", async () => {
    const fetchStub = (async () =>
      new Response(
        JSON.stringify({
          type: "urn:lyra:transport:invalid_request",
          detail: "bad request",
          requestId: "req_123",
        }),
        { status: 400, headers: { "Content-Type": "application/problem+json" } },
      )) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    await expect(transport.send(req("3", "runs.start"))).rejects.toMatchObject({
      name: "RpcTransportError",
      status: 400,
      requestId: "req_123",
      problemType: "urn:lyra:transport:invalid_request",
    } satisfies Partial<RpcTransportError>);
    await transport.close();
  });

  it("stays quiet when the stream is aborted (expected teardown, not an error)", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const responseFrame = frame({ jsonrpc: "2.0", id: "1", result: { runId: "run_01" } });
    const fetchStub = (async () => abortingSseResponse(responseFrame)) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("1", "runs.start"));
    await it.next(); // the response frame arrives; the next read aborts in the background
    await new Promise((resolve) => setTimeout(resolve, 0)); // let the aborted read settle
    await transport.close();

    expect(warn).not.toHaveBeenCalled();
  });

  it("a stream dying mid-run reports a typed stream termination", async () => {
    // Response frame arrives (runId run_01), then the connection dies with a
    // non-abort error — no segment.finished was ever delivered. Without the
    // transport event, every consumer of run_01's events would await forever.
    const responseFrame = frame({ jsonrpc: "2.0", id: "1", result: { runId: "run_01" } });
    const enc = new TextEncoder();
    let sent = false;
    const body = new ReadableStream<Uint8Array>({
      pull(controller) {
        if (!sent) {
          sent = true;
          controller.enqueue(enc.encode(responseFrame));
        } else {
          controller.error(new Error("connection reset")); // NOT an AbortError
        }
      },
    });
    const fetchStub = (async () =>
      new Response(body, {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      })) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("1", "runs.start"));
    const r0 = await it.next(); // the call's response
    const r1 = await it.next(); // the typed stream termination
    await transport.close();

    expect(r0.value).toMatchObject({
      type: "message",
      message: { id: "1", result: { runId: "run_01" } },
    });
    expect(r1.value).toMatchObject({
      type: "streamEnd",
      method: "runs.start",
      requestRpcId: "1",
      error: expect.any(RpcConnectionError),
    });
  });

  it("a stream ending before the call's response reports a request failure", async () => {
    // The POST opened but the stream EOS'd before the first (response) frame
    // — the pending call must reject, not hang forever.
    const fetchStub = (async () => sseResponse([])) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("7", "runs.start"));
    const r = await it.next();
    await transport.close();

    expect(r.value).toMatchObject({
      type: "requestError",
      rpcId: "7",
      error: expect.any(RpcTransportError),
    });
  });

  it("fails a stream carrying a malformed JSON-RPC envelope", async () => {
    const wire =
      frame({ jsonrpc: "2.0", id: "1", result: { runId: "run_01" } }) + `data: {not json}\n\n`;
    const fetchStub = (async () => sseResponse([wire])) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("1", "runs.start"));
    const response = await it.next();
    const ended = await it.next();
    await transport.close();

    expect(response.value).toMatchObject({ type: "message", message: { id: "1" } });
    expect(ended.value).toMatchObject({
      type: "streamEnd",
      method: "runs.start",
      requestRpcId: "1",
      error: expect.any(RpcTransportError),
    });
  });
});
