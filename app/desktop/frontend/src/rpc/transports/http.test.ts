import type { RpcNotification } from "../types";
import { describe, expect, it } from "vitest";
import { createHttpTransport } from "./http";

// A 200 text/event-stream Response whose body emits the given byte chunks.
function sseResponse(chunks: string[]): Response {
  const enc = new TextEncoder();
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(enc.encode(c));
      controller.close();
    },
  });
  return new Response(body, {
    status: 200,
    headers: { "Content-Type": "text/event-stream" },
  });
}

const sseFrame = (obj: unknown, id?: string): string =>
  `${id ? `id: ${id}\n` : ""}data: ${JSON.stringify(obj)}\n\n`;

describe("HTTPTransport SSE", () => {
  it("parses notifications, reassembling a frame split across chunks", async () => {
    const n1: RpcNotification = {
      jsonrpc: "2.0",
      method: "notifications.run.event",
      params: { a: 1 },
    };
    const n2: RpcNotification = {
      jsonrpc: "2.0",
      method: "notifications.run.event",
      params: { b: 2 },
    };
    // A keepalive comment + two framed events; sliced mid-stream so the
    // parser must buffer the partial frame across chunk boundaries.
    const wire = `: open\n\n${sseFrame(n1, "evt_1")}${sseFrame(n2, "evt_2")}`;
    const cut = Math.floor(wire.length / 2);

    const fetchStub = (async (url: string) => {
      if (String(url).includes("/rpc/stream")) {
        return sseResponse([wire.slice(0, cut), wire.slice(cut)]);
      }
      return new Response(null, { status: 204 });
    }) as unknown as typeof fetch;

    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const iterator = transport.recv()[Symbol.asyncIterator]();

    const first = await iterator.next();
    const second = await iterator.next();
    await transport.close();

    expect(first.value).toMatchObject({ params: { a: 1 } });
    expect(second.value).toMatchObject({ params: { b: 2 } });
  });

  it("skips malformed data without tearing down the stream", async () => {
    const good: RpcNotification = {
      jsonrpc: "2.0",
      method: "notifications.run.event",
      params: { ok: 1 },
    };
    const wire = `data: {not json}\n\n${sseFrame(good)}`;
    const fetchStub = (async (url: string) =>
      String(url).includes("/rpc/stream")
        ? sseResponse([wire])
        : new Response(null, { status: 204 })) as unknown as typeof fetch;

    const transport = createHttpTransport({ baseUrl: "http://x", fetch: fetchStub });
    const iterator = transport.recv()[Symbol.asyncIterator]();

    const next = await iterator.next();
    await transport.close();

    // The malformed frame is dropped; the well-formed one still arrives.
    expect(next.value).toMatchObject({ params: { ok: 1 } });
  });
});
