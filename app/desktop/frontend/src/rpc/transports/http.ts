// HTTPTransport — JSON-RPC over HTTP for the web frontend, using
// **streamable HTTP** (docs/protocol/TRANSPORT.md §6): a streaming method's POST
// response body IS its event stream. There is no separate notification
// connection — every server→client message rides the POST response it
// belongs to.
//
// send():  POST /v2/rpc, then branch on the response Content-Type
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
// stream opened (event-stream). The runtime reserves 204/202 for client
// notifications, but this closed SDK sends Requests only, so either is a protocol
// mismatch here. Any other status is a transport failure → RpcTransportError.

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
import { errorMessage, parseTransportProblem, RpcTransportError } from "../errors";
import {
  type Transport,
  type TransportEvent,
  type TransportRequest,
  type TransportResponseMetadata,
  type TransportSendOptions,
} from "../transport";
import type { RpcId } from "../types";
import { isResponse, parseRpcMessage } from "../types";
import { isWireStreamingMethodName, type WireStreamingMethodName } from "../wire.methods.generated";

const RPC_PATH = "/v2/rpc";

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
  /** Custom fetch impl (tests inject one). Defaults to globalThis.fetch. */
  fetch?: typeof fetch;
}

export function createHttpTransport(config: HttpTransportConfig): Transport {
  const baseUrl = config.baseUrl.replace(/\/+$/, "");
  const fetchImpl = config.fetch ?? globalThis.fetch.bind(globalThis);

  const channel = createPushPullChannel<TransportEvent>();
  const closeController = new AbortController();
  // Active SSE body readers — close() cancels in-flight streams through these.
  const readers = new Set<ReadableStreamDefaultReader<Uint8Array>>();

  function requestHeaders(extra: Record<string, string>): Record<string, string> {
    const headers: Record<string, string> = { ...extra };
    if (config.localToken) headers.Authorization = `Bearer ${config.localToken}`;
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
  // whether `requestId`'s response was delivered, then report a lifecycle event
  // owned by that exact request. It never impersonates a JSON-RPC response or
  // notification.
  //
  // For runtime.subscribe, which has no terminal frame, any non-abort end —
  // graceful EOS included — means "subscription over, resubscribe"
  // (AUX_API §3.1).
  async function drainStream(
    body: ReadableStream<Uint8Array>,
    requestId: RpcId,
    method: WireStreamingMethodName,
    metadata: TransportResponseMetadata,
    signal?: AbortSignal,
  ): Promise<void> {
    let responseSeen = false;
    let streamError: Error | undefined;
    const parser = createParser({
      onEvent(event) {
        if (!event.data) return;
        const msg = parseRpcMessage(event.data);
        if (!msg) {
          streamError = new RpcTransportError(
            "invalid JSON-RPC envelope in event stream",
            undefined,
            metadata.requestId,
          );
          throw streamError;
        }
        if (isResponse(msg) && msg.id === requestId) {
          responseSeen = true;
        }
        channel.push({ type: "message", message: msg, requestRpcId: requestId, metadata });
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
      parser.feed(decoder.decode());
    } catch (err) {
      // Aborts (stop / switch session / superseded run / transport close) are
      // expected teardown via the fetch signal — not failures, stay quiet.
      aborted = signal?.aborted === true || (err instanceof Error && err.name === "AbortError");
      if (!aborted && !channel.closed) {
        streamError = err instanceof Error ? err : new RpcTransportError(String(err));
      }
    } finally {
      readers.delete(reader);
      reader.releaseLock();
    }
    if (aborted || channel.closed) return;
    if (!responseSeen) {
      channel.push({
        type: "requestError",
        rpcId: requestId,
        error:
          streamError ??
          new RpcTransportError(
            "event stream ended before the call's response",
            undefined,
            metadata.requestId,
          ),
      });
    }
    channel.push({
      type: "streamEnd",
      method,
      requestRpcId: requestId,
      ...(streamError ? { error: streamError } : {}),
      metadata,
    });
  }

  async function send(
    msg: TransportRequest,
    signal?: AbortSignal,
    options: TransportSendOptions = {},
  ): Promise<void> {
    if (channel.closed) throw new RpcTransportError("transport closed");

    const method = msg.method;
    const url = `${baseUrl}${RPC_PATH}`;
    const rpcId = msg.id;
    const requestSignal = signal
      ? AbortSignal.any([signal, closeController.signal])
      : closeController.signal;

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
    if (options.idempotencyKey) headers["Idempotency-Key"] = options.idempotencyKey;
    if (options.lastEventId) headers["Last-Event-Id"] = options.lastEventId;
    // Write `traceparent` (+ baggage) for THIS span into the request headers.
    propagation.inject(trace.setSpan(context.active(), span), headers);

    let res: Response;
    try {
      res = await fetchImpl(url, {
        method: "POST",
        headers,
        body: JSON.stringify(msg),
        signal: requestSignal,
      });
    } catch (err) {
      endSpan(span, err);
      throw new RpcTransportError(`fetch failed: ${errorMessage(err)}`);
    }
    span.setAttribute("rpc.http.status_code", res.status);
    const metadata: TransportResponseMetadata = {
      requestId: res.headers.get("Request-Id") ?? undefined,
    };
    if (metadata.requestId) span.setAttribute("lyra.request_id", metadata.requestId);

    // This client sends Requests only; a bodyless notification acknowledgement is
    // therefore always a protocol mismatch.
    if (res.status === 204 || res.status === 202) {
      const err = new RpcTransportError(
        `http ${res.status}: RPC call ended without a response`,
        res.status,
        metadata.requestId,
      );
      endSpan(span, err);
      throw err;
    }

    // Any non-2xx is a transport-layer failure represented as Problem Details.
    if (!res.ok) {
      const text = await res.text().catch(() => "");
      const problem = parseTransportProblem(text);
      const requestId = problem?.requestId ?? metadata.requestId;
      const detail = problem?.detail || res.statusText || "transport request failed";
      const err = new RpcTransportError(
        `http ${res.status}: ${detail}${requestId ? ` (request ${requestId})` : ""}`,
        res.status,
        requestId,
        problem?.type,
      );
      endSpan(span, err);
      throw err;
    }

    // Streaming method (TRANSPORT.md §6.4): the body is this call's event
    // stream (response frame + notifications). Drain it in the background so
    // send() returns once headers are in, not at stream end.
    if ((res.headers.get("Content-Type") ?? "").includes("text/event-stream")) {
      if (!isWireStreamingMethodName(method)) {
        const err = new RpcTransportError(
          `non-streaming RPC method ${method} returned an event stream`,
          undefined,
          metadata.requestId,
        );
        endSpan(span, err);
        await res.body?.cancel();
        throw err;
      }
      if (!res.body) {
        const err = new RpcTransportError(
          "event-stream response has no body",
          undefined,
          metadata.requestId,
        );
        endSpan(span, err);
        throw err;
      }
      // A stream may drain for minutes; that wall-clock belongs to the run,
      // not the HTTP request span. The reader remains bound to requestSignal.
      endSpan(span);
      void drainStream(res.body, rpcId, method, metadata, requestSignal);
      return;
    }

    // Non-streaming: a single JSON-RPC message in the body. A malformed
    // envelope fails THIS call (rejected via send()'s caller) rather than
    // pushing garbage that never correlates and hangs the pending promise.
    let text: string;
    try {
      text = await res.text();
    } catch (cause) {
      const err = new RpcTransportError(
        `failed to read RPC response: ${errorMessage(cause)}`,
        undefined,
        metadata.requestId,
      );
      endSpan(span, err);
      throw err;
    }
    if (!text) {
      const err = new RpcTransportError(
        "RPC response body is empty",
        undefined,
        metadata.requestId,
      );
      endSpan(span, err);
      throw err;
    }
    const inbound = parseRpcMessage(text);
    if (!inbound) {
      const err = new RpcTransportError(
        `invalid JSON-RPC envelope in response body: ${text.slice(0, 200)}`,
        undefined,
        metadata.requestId,
      );
      endSpan(span, err);
      throw err;
    }
    if (!isResponse(inbound) || inbound.id !== rpcId) {
      const err = new RpcTransportError(
        "RPC response does not match the outbound request",
        undefined,
        metadata.requestId,
      );
      endSpan(span, err);
      throw err;
    }
    channel.push({ type: "message", message: inbound, requestRpcId: rpcId, metadata });
    endSpan(span);
  }

  function recv(): AsyncIterable<TransportEvent> {
    // RpcClient calls recv() once and consumes the iterator for the transport's
    // life; every inbound message arrives via a POST response (see send()).
    return channel.iterator();
  }

  async function close(): Promise<void> {
    channel.close();
    closeController.abort();
    for (const reader of readers) void reader.cancel().catch(() => {});
    readers.clear();
  }

  return { send, recv, close };
}
