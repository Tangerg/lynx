// HTTPTransport — JSON-RPC over HTTP for the web frontend, using
// **streamable HTTP** (docs/protocol/TRANSPORT.md §6): a streaming method's POST
// response body IS its event stream. There is no separate notification
// connection — every server→client message rides the POST response it
// belongs to.
//
// send():  POST /v2/rpc/{method}, then branch on the response Content-Type
//   - application/json   → one JSON-RPC message, pushed to the channel
//   - text/event-stream  → the call's response (first frame) + its
//                          notifications, drained frame-by-frame into the
//                          channel by a background reader (send() returns
//                          once headers are in, NOT at stream end)
// recv():  the merged inbound channel RpcClient consumes — responses
//   correlate by JSON-RPC id, notifications route by method. Fed entirely by
//   the POST responses above; there is no GET stream.
//
// SSE wire framing is delegated to eventsource-parser. Reconnection is a
// per-run concern (runs.subscribe + Last-Event-Id, TRANSPORT.md §9.2) handled
// above the transport — there is no standing-connection reconnect loop here.
//
// HTTP status (docs/protocol/TRANSPORT.md §6.3): 200 = JSON-RPC response (json) or
// stream opened (event-stream); 204/202 = notification ack; any other status
// = transport-layer failure → RpcTransportError.

import {
  context,
  propagation,
  type Span,
  SpanKind,
  SpanStatusCode,
  trace,
} from "@opentelemetry/api";
import { createParser } from "eventsource-parser";
import { createPushPullChannel } from "../channel";
import { RpcTransportError } from "../errors";
import { STREAM_DOWN_METHOD, WORKSPACE_SUBSCRIBE_METHOD, type Transport } from "../transport";
import type { RpcId, RpcMessage } from "../types";
import { JSONRPC_VERSION } from "../types";

// Delegating tracer — resolves to the global provider once observability is
// installed (no-op spans before then). One CLIENT span per RPC call; the
// W3C `traceparent` it injects extends the backend's existing OTel trace
// (TRANSPORT.md §2: trace context rides headers, never the JSON-RPC body).
const tracer = trace.getTracer("lyra-frontend");

function endSpan(span: Span, err?: unknown): void {
  if (err !== undefined) {
    span.setStatus({
      code: SpanStatusCode.ERROR,
      message: err instanceof Error ? err.message : String(err),
    });
  }
  span.end();
}

export interface HttpTransportConfig {
  /** Runtime base URL, e.g. "http://127.0.0.1:17171". No trailing slash. */
  baseUrl: string;
  /**
   * Local-loopback process gate token (read from `~/.lyra/local-token` by the
   * host shell, passed in here). Sent as `Authorization: Bearer`. Not a
   * user-auth credential — see docs/protocol/TRANSPORT.md §11.
   */
  localToken?: string;
  /** Negotiated protocol version, sent as `X-Protocol-Version` on every
   *  request (TRANSPORT.md §2 / §6.2 canonical request shape). */
  protocolVersion?: string;
  /** Custom fetch impl (tests inject one). Defaults to globalThis.fetch. */
  fetch?: typeof fetch;
}

export function createHttpTransport(config: HttpTransportConfig): Transport {
  const baseUrl = config.baseUrl.replace(/\/$/, "");
  const fetchImpl = config.fetch ?? globalThis.fetch.bind(globalThis);

  const channel = createPushPullChannel<RpcMessage>();
  // Active SSE body readers — close() cancels in-flight streams through these.
  const readers = new Set<ReadableStreamDefaultReader<Uint8Array>>();

  function requestHeaders(extra: Record<string, string>): Record<string, string> {
    const headers: Record<string, string> = { ...extra };
    if (config.localToken) headers.Authorization = `Bearer ${config.localToken}`;
    if (config.protocolVersion) headers["X-Protocol-Version"] = config.protocolVersion;
    return headers;
  }

  // Drain a text/event-stream POST response into the channel. Runs detached
  // (a run may stream for minutes); not awaited by send(). The first frame is
  // the call's JSON-RPC response (correlated by id upstream), the rest are
  // `notifications.run.event` frames. SSE framing → eventsource-parser.
  //
  // A stream that dies any way OTHER than a caller abort (network drop,
  // runtime restart, premature EOS) must not strand its consumers: without a
  // signal, the call whose response never arrived hangs its pending promise
  // forever, and a run mid-stream leaves the UI stuck "running". So we sniff
  // what this stream carried — whether `requestId`'s response was delivered,
  // and which runIds streamed events here — and on an abnormal end synthesize
  // (a) an error Response settling the un-responded call, and (b) a
  // STREAM_DOWN notification (transport.ts) so rpc/stream.ts closes the
  // affected runs' channels. A unary call's stream EOSing after its one
  // response frame produces neither (responseSeen, no runIds) — that's the
  // normal case, not a death.
  //
  // workspace.subscribe is the one NON-run stream: it has no runId to
  // attribute and no terminal frame, so for it ANY non-abort end — graceful
  // EOS included — means "subscription over, resubscribe" (AUX_API §3.1). We
  // signal that with a method-attributed STREAM_DOWN.
  async function drainStream(
    body: ReadableStream<Uint8Array>,
    requestId?: RpcId,
    method?: string,
  ): Promise<void> {
    let responseSeen = false;
    const runIds = new Set<string>();
    const parser = createParser({
      onEvent(event) {
        if (!event.data) return;
        try {
          const msg = JSON.parse(event.data) as RpcMessage;
          const m = msg as {
            id?: RpcId;
            method?: string;
            params?: { runId?: string };
            result?: { runId?: string };
          };
          if (requestId !== undefined && m.id === requestId) {
            responseSeen = true;
            // runs.start/resume/subscribe responses carry the stream's root
            // runId — record it so a death BEFORE the first run event still
            // names the run in the STREAM_DOWN synthetic.
            if (typeof m.result?.runId === "string") runIds.add(m.result.runId);
          }
          if (m.method === "notifications.run.event" && typeof m.params?.runId === "string") {
            runIds.add(m.params.runId);
          }
          channel.push(msg);
        } catch {
          // Malformed frame — skip rather than tearing down the stream.
        }
      },
    });
    const reader = body.getReader();
    readers.add(reader);
    const decoder = new TextDecoder();
    let aborted = false;
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        parser.feed(decoder.decode(value, { stream: true }));
      }
    } catch (err) {
      // Aborts (stop / switch session / superseded run / transport close) are
      // expected teardown via the fetch signal — not failures, stay quiet.
      aborted = err instanceof Error && err.name === "AbortError";
      if (!aborted && !channel.closed) {
        console.warn("[rpc] stream read error:", (err as Error).message);
      }
    } finally {
      readers.delete(reader);
    }
    if (aborted || channel.closed) return;
    if (requestId !== undefined && !responseSeen) {
      channel.push({
        jsonrpc: JSONRPC_VERSION,
        id: requestId,
        error: { code: -32000, message: "transport: stream ended before the call's response" },
      } as RpcMessage);
    }
    if (runIds.size > 0 || method === WORKSPACE_SUBSCRIBE_METHOD) {
      channel.push({
        jsonrpc: JSONRPC_VERSION,
        method: STREAM_DOWN_METHOD,
        params: { runIds: [...runIds], method },
      } as RpcMessage);
    }
  }

  async function send(msg: RpcMessage, signal?: AbortSignal): Promise<void> {
    if (channel.closed) throw new RpcTransportError("transport closed");

    // Single URL form: POST /v2/rpc/{method}. Response messages don't carry a
    // method and HTTPTransport never sends them (the server issues responses
    // as the first frame of the method's own stream).
    const method = "method" in msg ? msg.method : undefined;
    if (!method) {
      throw new RpcTransportError(
        "HTTP transport only sends Request / Notification messages (which carry a `method`)",
      );
    }
    const url = `${baseUrl}/v2/rpc/${method}`;

    // CLIENT span for this call. Created synchronously before the first await
    // so its parent is whatever context is active at the call site (the run
    // span, when useAgentSession wrapped the dispatch) — see lib/observability.
    const span = tracer.startSpan(`rpc ${method}`, {
      kind: SpanKind.CLIENT,
      attributes: { "rpc.system": "jsonrpc", "rpc.method": method },
    });
    const headers = requestHeaders({
      "Content-Type": "application/json",
      Accept: "application/json, text/event-stream",
    });
    // Write `traceparent` (+ baggage) for THIS span into the request headers.
    propagation.inject(trace.setSpan(context.active(), span), headers);

    let res: Response;
    try {
      res = await fetchImpl(url, { method: "POST", headers, body: JSON.stringify(msg), signal });
    } catch (err) {
      endSpan(span, err);
      throw new RpcTransportError(`fetch failed: ${(err as Error).message}`);
    }
    span.setAttribute("rpc.http.status_code", res.status);

    // 204/202 = notification accepted; no body (TRANSPORT.md §6.3).
    if (res.status === 204 || res.status === 202) {
      endSpan(span);
      return;
    }

    // Any non-2xx is a transport-layer failure (flat text, not an envelope).
    if (!res.ok) {
      const text = await res.text().catch(() => "");
      const err = new RpcTransportError(`http ${res.status}: ${text}`, res.status);
      endSpan(span, err);
      throw err;
    }

    // The call succeeded. End the CLIENT span here — a streaming method's body
    // may drain for minutes, but that wall-clock belongs to the run span, not
    // to this request span.
    endSpan(span);

    // Streaming method (TRANSPORT.md §6.4): the body is this call's event
    // stream (response frame + notifications). Drain it in the background so
    // send() returns once headers are in, not at stream end.
    if ((res.headers.get("Content-Type") ?? "").includes("text/event-stream")) {
      if (!res.body) throw new RpcTransportError("event-stream response has no body");
      void drainStream(res.body, "id" in msg ? msg.id : undefined, method);
      return;
    }

    // Non-streaming: a single JSON-RPC message in the body.
    const text = await res.text();
    if (!text) return; // empty body is acceptable for some acks
    try {
      channel.push(JSON.parse(text) as RpcMessage);
    } catch (err) {
      throw new RpcTransportError(`invalid JSON in response body: ${(err as Error).message}`);
    }
  }

  function recv(): AsyncIterable<RpcMessage> {
    // RpcClient calls recv() once and consumes the iterator for the transport's
    // life; every inbound message arrives via a POST response (see send()).
    return channel.iterator();
  }

  async function close(): Promise<void> {
    channel.close();
    for (const reader of readers) void reader.cancel().catch(() => {});
    readers.clear();
  }

  return { send, recv, close };
}
