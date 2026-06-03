// HTTPTransport — JSON-RPC over HTTP for the Web frontend (and any
// future remote/facade scenarios). See docs/TRANSPORT.md §6-§9.
//
// Wire endpoints:
//   - POST /v2/rpc/{method}   — single form, no bare /v2/rpc fallback
//   - GET  /v2/rpc/stream     — SSE stream of inbound notifications
//
// The notification stream is consumed via `fetch` + a ReadableStream reader,
// NOT the native `EventSource`. EventSource is GET-only and can't set request
// headers, which forced the connection id into a `?conn=` query and left the
// reconnect behaviour opaque. The fetch reader sends `X-Conn-Id`
// (+ Authorization + Last-Event-Id) as real headers — identical to the POST
// path — and gives full control over parsing + reconnect.
//
// HTTP status mapping (docs/TRANSPORT.md §6.3):
//   200         → JSON-RPC Response in body (may carry business error)
//   202         → client Notification accepted; no body
//   400/404/409 → transport-layer failure
//   401/500/503 → flat JSON — surfaced as RpcTransportError

import { createPushPullChannel } from "../channel";
import { RpcTransportError } from "../errors";
import type { Transport } from "../transport";
import type { RpcMessage } from "../types";

const SSE_RECONNECT_MS = 1000;

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
}

export function createHttpTransport(config: HttpTransportConfig): Transport {
  const baseUrl = config.baseUrl.replace(/\/$/, "");
  const fetchImpl = config.fetch ?? globalThis.fetch.bind(globalThis);

  const channel = createPushPullChannel<RpcMessage>();
  let lastEventId: string | null = null;

  // Connection id ties this client's POSTs to its notification stream
  // (TRANSPORT.md §2 / §8). Sent as the `X-Conn-Id` header on BOTH the POSTs
  // and the SSE GET (plus a `?conn=` query — EventSource-style backends).
  const connId = crypto.randomUUID();

  let sseAbort: AbortController | null = null;
  let sseStarted = false;

  function authHeaders(extra: Record<string, string>): Record<string, string> {
    const headers: Record<string, string> = { "X-Conn-Id": connId, ...extra };
    if (config.localToken) headers.Authorization = `Bearer ${config.localToken}`;
    if (lastEventId) headers["Last-Event-Id"] = lastEventId;
    return headers;
  }

  function openSse(): void {
    if (sseStarted || channel.closed) return;
    sseStarted = true;
    sseAbort = new AbortController();
    void pumpSse();
  }

  // Connect the notification stream and keep it alive across drops until the
  // transport closes. Each connection streams `notifications/*` frames; a
  // clean end or a network error reconnects (with Last-Event-Id resume).
  async function pumpSse(): Promise<void> {
    while (!channel.closed && !sseAbort?.signal.aborted) {
      try {
        const res = await fetchImpl(`${baseUrl}/v2/rpc/stream?conn=${connId}`, {
          method: "GET",
          headers: authHeaders({ Accept: "text/event-stream" }),
          signal: sseAbort?.signal,
        });
        if (!res.ok || !res.body) {
          throw new RpcTransportError(`SSE connect failed: http ${res.status}`, res.status);
        }
        await readSse(res.body);
      } catch (err) {
        if (channel.closed || sseAbort?.signal.aborted) return;
        console.warn("[rpc] SSE dropped, reconnecting:", (err as Error).message);
      }
      if (channel.closed || sseAbort?.signal.aborted) return;
      await delay(SSE_RECONNECT_MS);
    }
  }

  // Read an SSE response body, splitting on frame boundaries (a blank line)
  // and buffering across chunks so a frame split mid-stream is reassembled.
  async function readSse(body: ReadableStream<Uint8Array>): Promise<void> {
    const reader = body.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    for (;;) {
      const { done, value } = await reader.read();
      if (done) return;
      buf += decoder.decode(value, { stream: true });
      const frames = buf.split(/\r\n\r\n|\n\n|\r\r/);
      buf = frames.pop() ?? ""; // trailing partial frame stays buffered
      for (const frame of frames) handleFrame(frame);
    }
  }

  // Parse one SSE frame: concatenate `data:` lines, track `id:`; push the
  // assembled JSON into the channel. `event:` / `retry:` are ignored — our
  // notifications are unnamed `data:` frames.
  function handleFrame(frame: string): void {
    let data = "";
    for (const raw of frame.split(/\r\n|\n|\r/)) {
      if (!raw || raw.startsWith(":")) continue; // blank / comment (keepalive)
      const colon = raw.indexOf(":");
      const field = colon === -1 ? raw : raw.slice(0, colon);
      let val = colon === -1 ? "" : raw.slice(colon + 1);
      if (val.startsWith(" ")) val = val.slice(1);
      if (field === "data") data += data ? `\n${val}` : val;
      else if (field === "id") lastEventId = val;
    }
    if (!data) return;
    try {
      channel.push(JSON.parse(data) as RpcMessage);
    } catch {
      // Malformed frame — skip rather than tearing down the stream.
    }
  }

  async function send(msg: RpcMessage, signal?: AbortSignal): Promise<void> {
    if (channel.closed) throw new RpcTransportError("transport closed");

    // Single URL form: POST /v2/rpc/{method}. Greenfield — no fallback
    // to bare /v2/rpc. Response messages don't carry method and
    // HTTPTransport never sends them (server issues Responses via SSE).
    const method = "method" in msg ? msg.method : undefined;
    if (!method) {
      throw new RpcTransportError(
        "HTTP transport only sends Request / Notification messages (which carry a `method`)",
      );
    }
    const url = `${baseUrl}/v2/rpc/${method}`;

    let res: Response;
    try {
      res = await fetchImpl(url, {
        method: "POST",
        headers: authHeaders({ "Content-Type": "application/json" }),
        body: JSON.stringify(msg),
        signal,
      });
    } catch (err) {
      throw new RpcTransportError(`fetch failed: ${(err as Error).message}`);
    }

    // 202 = client Notification accepted; no body (TRANSPORT.md §6.3).
    // 204 tolerated for backends that use it for the same ack.
    if (res.status === 202 || res.status === 204) return;

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
    // Lazy SSE open: first recv() call starts the notification stream.
    // RpcClient calls recv() once and keeps the iterator forever.
    openSse();
    return channel.iterator();
  }

  async function close(): Promise<void> {
    channel.close();
    sseAbort?.abort();
    sseAbort = null;
  }

  return { send, recv, close };
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
