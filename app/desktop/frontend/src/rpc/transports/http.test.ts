import type { RpcMessage } from "../types";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RpcTransportError } from "../errors";
import { STREAM_DOWN_METHOD } from "../transport";
import { createHttpTransport } from "./http";

afterEach(() => vi.restoreAllMocks());

// A 200 text/event-stream POST response whose body emits the given chunks.
function sseResponse(chunks: string[]): Response {
  const enc = new TextEncoder();
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(enc.encode(c));
      controller.close();
    },
  });
  return new Response(body, { status: 200, headers: { "Content-Type": "text/event-stream" } });
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

function jsonResponse(obj: unknown): Response {
  return new Response(JSON.stringify(obj), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// One SSE frame. `id` omitted ⇒ a response/ack frame (no SSE id); set ⇒ an
// event frame carrying its eventId.
const frame = (obj: unknown, id?: string): string =>
  `${id ? `id: ${id}\n` : ""}data: ${JSON.stringify(obj)}\n\n`;

const req = (id: string, method: string): RpcMessage =>
  ({ jsonrpc: "2.0", id, method, params: {} }) as RpcMessage;

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
      sseResponse([wire.slice(0, cut), wire.slice(cut)])) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("1", "runs.start"));
    const r0 = await it.next();
    const r1 = await it.next();
    const r2 = await it.next();
    await transport.close();

    expect(r0.value).toMatchObject({ id: "1", result: { runId: "run_01" } });
    expect(r1.value).toMatchObject({ params: { event: { type: "segment.started" } } });
    expect(r2.value).toMatchObject({ params: { event: { type: "segment.finished" } } });
  });

  it("non-streaming method: POST returns a single application/json message", async () => {
    const fetchStub = vi.fn(async () =>
      jsonResponse({
        jsonrpc: "2.0",
        id: "2",
        result: { id: "ses_1" },
      }),
    );
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("2", "sessions.get"));
    const r = await it.next();
    await transport.close();

    expect(r.value).toMatchObject({ id: "2", result: { id: "ses_1" } });
    expect(fetchStub).toHaveBeenCalledWith(
      "http://x/v2/rpc",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("sends the logical mutation idempotency key as transport metadata", async () => {
    const fetchStub = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
      jsonResponse({ jsonrpc: "2.0", id: "2", result: { id: "ses_1" } }),
    );
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("2", "sessions.create"), undefined, {
      idempotencyKey: "operation-key-1",
    });
    await it.next();
    await transport.close();

    const headers = fetchStub.mock.calls[0]?.[1]?.headers as Record<string, string>;
    expect(headers["Idempotency-Key"]).toBe("operation-key-1");
  });

  it("204 notification ack is a no-op", async () => {
    const fetchStub = (async () => new Response(null, { status: 204 })) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    await expect(
      transport.send({ jsonrpc: "2.0", method: "test.notification", params: {} } as RpcMessage),
    ).resolves.toBeUndefined();
    await transport.close();
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

  it("a stream dying mid-run synthesizes a stream-down naming the run", async () => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
    // Response frame arrives (runId run_01), then the connection dies with a
    // non-abort error — no segment.finished was ever delivered. Without the
    // synthetic, every consumer of run_01's events would await forever.
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
    const r1 = await it.next(); // the synthetic stream-down
    await transport.close();

    expect(r0.value).toMatchObject({ id: "1", result: { runId: "run_01" } });
    expect(r1.value).toMatchObject({ method: STREAM_DOWN_METHOD, params: { runIds: ["run_01"] } });
  });

  it("a stream ending before the call's response synthesizes an error Response", async () => {
    // The POST opened but the stream EOS'd before the first (response) frame
    // — the pending call must reject, not hang forever.
    const fetchStub = (async () => sseResponse([])) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("7", "runs.start"));
    const r = await it.next();
    await transport.close();

    expect(r.value).toMatchObject({ id: "7", error: { code: -32000 } });
  });

  it("skips a malformed frame without tearing down the stream", async () => {
    const wire =
      `data: {not json}\n\n` +
      frame({ jsonrpc: "2.0", method: "notifications.run.event", params: { ok: 1 } }, "evt_1");
    const fetchStub = (async () => sseResponse([wire])) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("1", "runs.start"));
    const r = await it.next();
    await transport.close();

    expect(r.value).toMatchObject({ params: { ok: 1 } });
  });
});
