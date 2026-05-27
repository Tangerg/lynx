// HTTPTransport — JSON-RPC over HTTP for the Web frontend (and any
// future remote/facade scenarios). See docs/TRANSPORT.md §4.3 +
// docs/API.md §10.
//
// Two endpoints:
//   - POST /v1/rpc/{method}   (recommended) — request/notification
//   - GET  /v1/rpc/stream     — SSE stream of inbound notifications
//
// Servers MUST also accept POST /v1/rpc (without method suffix); we
// prefer the suffixed form for ops visibility but fall back to plain
// /v1/rpc when the message has no method (defensive — shouldn't
// happen with a well-formed Message).
//
// HTTP status mapping (docs/API.md §7.3):
//   200/204   → ack received, JSON-RPC Response (if any) in body
//   400/404/409 → JSON-RPC error envelope in body
//   401/500/503 → flat JSON {error, traceId?} — surfaced as RpcTransportError

import { RpcTransportError } from "../errors";
import type { Transport } from "../transport";
import type { RpcMessage, RpcNotification } from "../types";
import { JSONRPC_VERSION, isNotification } from "../types";

export interface HttpTransportConfig {
  /** Runtime base URL, e.g. "http://127.0.0.1:17171". No trailing slash. */
  baseUrl: string;
  /**
   * Local-loopback process gate token (read from `~/.lyra/local-token` by
   * the host shell, passed in here). Sent as Authorization: Bearer.
   * Not a user-auth credential — see docs/API.md §1.2.
   */
  localToken?: string;
  /** Custom fetch impl (tests inject one). Defaults to globalThis.fetch. */
  fetch?: typeof fetch;
  /** Custom EventSource ctor (tests inject one). Defaults to global. */
  EventSource?: typeof EventSource;
}

export function createHttpTransport(config: HttpTransportConfig): Transport {
  const baseUrl = config.baseUrl.replace(/\/$/, "");
  const fetchImpl = config.fetch ?? globalThis.fetch.bind(globalThis);
  const EventSourceImpl = config.EventSource ?? globalThis.EventSource;

  let sse: EventSource | null = null;
  let lastEventId: string | null = null;
  let closed = false;

  // Pending recv readers: each call to recv() opens an SSE stream and
  // yields inbound notifications. The SSE channel is the only inbound
  // path — JSON-RPC Responses to outbound Requests come back inline as
  // the POST response (we feed them to recv() via the same queue).
  const pending: RpcMessage[] = [];
  let waiters: Array<(msg: RpcMessage | typeof DONE) => void> = [];
  const DONE = Symbol("http-transport-done");
  type Sentinel = typeof DONE;

  function emit(msg: RpcMessage | Sentinel): void {
    if (waiters.length > 0) {
      const next = waiters.shift()!;
      next(msg);
    } else if (msg !== DONE) {
      pending.push(msg);
    }
  }

  function openSse(): void {
    if (sse || closed) return;
    const url = `${baseUrl}/v1/rpc/stream`;
    sse = new EventSourceImpl(url, { withCredentials: false });
    sse.onmessage = (ev) => {
      if (ev.lastEventId) lastEventId = ev.lastEventId;
      try {
        const msg = JSON.parse(ev.data) as RpcMessage;
        emit(msg);
      } catch {
        // Malformed event — skip rather than tearing down the stream.
      }
    };
    sse.onerror = () => {
      // Browser will auto-reconnect; nothing to do. EventSource sends
      // Last-Event-Id on reconnect automatically.
    };
  }

  function buildUrl(method?: string): string {
    return method ? `${baseUrl}/v1/rpc/${method}` : `${baseUrl}/v1/rpc`;
  }

  async function send(msg: RpcMessage, signal?: AbortSignal): Promise<void> {
    if (closed) throw new RpcTransportError("transport closed");

    // Method is in body always; URL suffix is the recommended ops form.
    const method = "method" in msg ? msg.method : undefined;
    const url = buildUrl(method);

    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (config.localToken) headers.Authorization = `Bearer ${config.localToken}`;
    if (lastEventId) headers["Last-Event-Id"] = lastEventId;

    let res: Response;
    try {
      res = await fetchImpl(url, {
        method: "POST",
        headers,
        body: JSON.stringify(msg),
        signal,
      });
    } catch (err) {
      throw new RpcTransportError(`fetch failed: ${(err as Error).message}`);
    }

    // 204 = Notification ack; nothing in body.
    if (res.status === 204) return;

    // 401/500/503 are flat JSON, not envelope (docs/API.md §7.3).
    if (res.status === 401 || res.status === 500 || res.status === 503) {
      const text = await res.text().catch(() => "");
      throw new RpcTransportError(`http ${res.status}: ${text}`, res.status);
    }

    // 200/400/404/409 should all carry a JSON-RPC envelope. Feed the
    // response into recv() so RpcClient correlates by id.
    if (res.status === 200 || res.status === 400 || res.status === 404 || res.status === 409) {
      const text = await res.text();
      if (!text) return; // empty body is acceptable for some Notifications
      try {
        const reply = JSON.parse(text) as RpcMessage;
        emit(reply);
      } catch (err) {
        throw new RpcTransportError(`invalid JSON in response body: ${(err as Error).message}`);
      }
      return;
    }

    // Anything else is unexpected — surface so the caller can react.
    const text = await res.text().catch(() => "");
    throw new RpcTransportError(`unexpected http ${res.status}: ${text}`, res.status);
  }

  async function* recv(): AsyncIterable<RpcMessage> {
    // First recv() opens the SSE stream lazily. RpcClient typically
    // calls recv() once and keeps the iterator forever.
    openSse();
    while (!closed || pending.length > 0) {
      const buffered = pending.shift();
      if (buffered !== undefined) {
        yield buffered;
        continue;
      }
      const next = await new Promise<RpcMessage | Sentinel>((resolve) => {
        waiters.push(resolve);
      });
      if (next === DONE) return;
      yield next;
    }
  }

  async function close(): Promise<void> {
    closed = true;
    if (sse) {
      sse.close();
      sse = null;
    }
    const pendingWaiters = waiters;
    waiters = [];
    for (const w of pendingWaiters) w(DONE);
  }

  return { send, recv, close };
}

// Helper for callers that only want to fire a Notification (no response
// expected). Frees them from constructing the RpcNotification by hand.
export function buildNotification<P>(method: string, params?: P): RpcNotification<P> {
  return { jsonrpc: JSONRPC_VERSION, method, params };
}

// Re-export the discriminator so transport implementers don't have to
// reach into ../types for it.
export { isNotification };
