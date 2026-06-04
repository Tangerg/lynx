import type { RpcMessage } from "../types";
import { describe, expect, it } from "vitest";
import { RpcTransportError } from "../errors";
import { createHttpTransport } from "./http";

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
        params: { event: { type: "run.started" } },
      },
      "evt_0001",
    );
    const finished = frame(
      {
        jsonrpc: "2.0",
        method: "notifications.run.event",
        params: { event: { type: "run.finished" } },
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
    expect(r1.value).toMatchObject({ params: { event: { type: "run.started" } } });
    expect(r2.value).toMatchObject({ params: { event: { type: "run.finished" } } });
  });

  it("non-streaming method: POST returns a single application/json message", async () => {
    const fetchStub = (async () =>
      jsonResponse({
        jsonrpc: "2.0",
        id: "2",
        result: { id: "ses_1" },
      })) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const it = transport.recv()[Symbol.asyncIterator]();

    await transport.send(req("2", "sessions.get"));
    const r = await it.next();
    await transport.close();

    expect(r.value).toMatchObject({ id: "2", result: { id: "ses_1" } });
  });

  it("204 notification ack is a no-op", async () => {
    const fetchStub = (async () => new Response(null, { status: 204 })) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    await expect(
      transport.send({ jsonrpc: "2.0", method: "runtime.shutdown", params: {} } as RpcMessage),
    ).resolves.toBeUndefined();
    await transport.close();
  });

  it("non-2xx surfaces a RpcTransportError", async () => {
    const fetchStub = (async () =>
      new Response("bad request", { status: 400 })) as unknown as typeof fetch;
    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    await expect(transport.send(req("3", "runs.start"))).rejects.toBeInstanceOf(RpcTransportError);
    await transport.close();
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
