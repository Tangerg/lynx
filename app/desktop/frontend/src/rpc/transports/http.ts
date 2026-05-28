// HTTPTransport — JSON-RPC over HTTP for the Web frontend (and any
// future remote/facade scenarios). See docs/TRANSPORT.md §4.3 +
// docs/API.md §10.
//
// Wire endpoints:
//   - POST /v1/rpc/{method}   — single form, no bare /v1/rpc fallback
//   - GET  /v1/rpc/stream     — SSE stream of inbound notifications
//
// HTTP status mapping (docs/API.md §7.3):
//   200/204     → ack received, JSON-RPC Response (if any) in body
//   400/404/409 → JSON-RPC error envelope in body
//   401/500/503 → flat JSON {error, traceId?} — surfaced as RpcTransportError

import { createPushPullChannel } from "../channel";
import { RpcTransportError } from "../errors";
import type { Transport } from "../transport";
import type { RpcMessage } from "../types";

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

  const channel = createPushPullChannel<RpcMessage>();
  let sse: EventSource | null = null;
  let lastEventId: string | null = null;

  function openSse(): void {
    if (sse || channel.closed) return;
    sse = new EventSourceImpl(`${baseUrl}/v1/rpc/stream`, { withCredentials: false });
    sse.onmessage = (ev) => {
      if (ev.lastEventId) lastEventId = ev.lastEventId;
      try {
        channel.push(JSON.parse(ev.data) as RpcMessage);
      } catch {
        // Malformed event — skip rather than tearing down the stream.
      }
    };
    sse.onerror = () => {
      // Browser will auto-reconnect; nothing to do. EventSource sends
      // Last-Event-Id on reconnect automatically.
    };
  }

  async function send(msg: RpcMessage, signal?: AbortSignal): Promise<void> {
    if (channel.closed) throw new RpcTransportError("transport closed");

    // Single URL form: POST /v1/rpc/{method}. Greenfield — no fallback
    // to bare /v1/rpc. Response messages don't carry method and
    // HTTPTransport never sends them (server issues Responses via SSE).
    const method = "method" in msg ? msg.method : undefined;
    if (!method) {
      throw new RpcTransportError(
        "HTTP transport only sends Request / Notification messages (which carry a `method`)",
      );
    }
    const url = `${baseUrl}/v1/rpc/${method}`;

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

    // 200/400/404/409 carry a JSON-RPC envelope. Feed the response into
    // the channel so RpcClient correlates by id.
    if (res.status === 200 || res.status === 400 || res.status === 404 || res.status === 409) {
      const text = await res.text();
      if (!text) return; // empty body is acceptable for some Notifications
      try {
        channel.push(JSON.parse(text) as RpcMessage);
      } catch (err) {
        throw new RpcTransportError(`invalid JSON in response body: ${(err as Error).message}`);
      }
      return;
    }

    // Anything else is unexpected — surface so the caller can react.
    const text = await res.text().catch(() => "");
    throw new RpcTransportError(`unexpected http ${res.status}: ${text}`, res.status);
  }

  function recv(): AsyncIterable<RpcMessage> {
    // Lazy SSE open: first recv() call triggers the connection. RpcClient
    // typically calls recv() once and keeps the iterator forever.
    openSse();
    return channel.iterator();
  }

  async function close(): Promise<void> {
    channel.close();
    if (sse) {
      sse.close();
      sse = null;
    }
  }

  return { send, recv, close };
}
