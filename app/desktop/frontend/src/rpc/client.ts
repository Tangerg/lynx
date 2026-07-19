// Typed JSON-RPC client wrapping a Transport. Owns id allocation,
// response correlation, notification dispatch. See docs/protocol/API.md §1.
//
// Correlation pattern borrowed from kimi-code's controlledPromise idea:
// each Request creates a pending entry with resolve/reject handles;
// the recv() loop pops the entry by id and settles the promise.
// Notifications go through subscribe() — no id, no waiter.

import { RpcError, RpcTransportError } from "./errors";
import type { RequestMeta } from "./shapes";
import type { Transport } from "./transport";
import type {
  RpcId,
  RpcMessage,
  RpcNotification,
  RpcRequest,
  RpcResponseError,
  RpcResponseSuccess,
} from "./types";
import { JSONRPC_VERSION, isErrorResponse, isNotification, isResponse } from "./types";

export type NotificationHandler = (msg: RpcNotification) => void;

export interface RpcClientOptions {
  requestMeta?: () => RequestMeta | undefined;
}

export interface RpcCallOptions {
  signal?: AbortSignal;
  idempotencyKey?: string;
}

export interface RpcClient {
  /** Send a Request and resolve with its result, or reject with RpcError. */
  call<R = unknown, P = unknown>(method: string, params?: P, options?: RpcCallOptions): Promise<R>;
  /** Send a Notification (fire-and-forget). */
  notify<P = unknown>(method: string, params?: P): Promise<void>;
  /** Subscribe to inbound notifications matching `method`. Returns an unsubscribe fn. */
  subscribe(method: string, handler: NotificationHandler): () => void;
  /** Tear down the client + underlying transport. */
  close(): Promise<void>;
}

interface Pending {
  resolve: (value: unknown) => void;
  reject: (err: unknown) => void;
}

export function createRpcClient(transport: Transport, options: RpcClientOptions = {}): RpcClient {
  // Monotonic integer counter, stringified at allocation — the wire id is
  // always a string (RpcId, §1.1), but an integer counter is the cheapest
  // way to keep every in-flight request's id unique so responses correlate.
  let nextId = 1;
  const pending = new Map<RpcId, Pending>();
  // method → handlers. We allow multiple subscribers per method so multiple
  // UI consumers can listen to the same stream.
  const subscribers = new Map<string, Set<NotificationHandler>>();
  let closed = false;

  function failAllPending(failure: RpcTransportError): void {
    for (const { reject } of pending.values()) reject(failure);
    pending.clear();
  }

  function failConnection(failure: RpcTransportError): void {
    closed = true;
    failAllPending(failure);
    subscribers.clear();
  }

  // Long-running pump that drains the transport's recv() into pending
  // promises + subscribers. When the stream ends — whether it throws or
  // closes cleanly — no further Responses can arrive, so every in-flight
  // request must be settled (rejected). Handling only the throw path left
  // pending calls hung forever on a clean EOS (e.g. a future WebSocket /
  // InProcess transport whose recv() ends without an exception).
  void (async () => {
    try {
      for await (const msg of transport.recv()) {
        dispatchInbound(msg);
      }
      failConnection(new RpcTransportError("transport stream ended"));
    } catch (err) {
      failConnection(new RpcTransportError(`transport recv() failed: ${(err as Error).message}`));
    }
  })();

  function dispatchInbound(msg: RpcMessage): void {
    if (isResponse(msg)) {
      const entry = pending.get(msg.id);
      if (!entry) return; // unsolicited or already settled — drop silently
      pending.delete(msg.id);
      if (isErrorResponse(msg)) {
        entry.reject(new RpcError((msg as RpcResponseError).error));
      } else {
        entry.resolve((msg as RpcResponseSuccess).result);
      }
      return;
    }
    if (isNotification(msg)) {
      const handlers = subscribers.get(msg.method);
      if (!handlers) return;
      for (const handler of handlers) {
        try {
          handler(msg);
        } catch (err) {
          // Subscribers must not crash the dispatch loop. Log and move on.
          console.error(`[rpc] notification handler for "${msg.method}" threw:`, err);
        }
      }
      return;
    }
    // Unexpected: server-initiated Requests are not in our protocol.
    // Drop them — see docs/protocol/API.md §1.1 (we don't do server→client RPC).
    console.warn("[rpc] dropping unexpected server-initiated Request", msg);
  }

  function paramsWithMeta<P>(params: P | undefined): unknown {
    const meta = options.requestMeta?.();
    if (!meta) return params as P;
    if (params === undefined) return { _meta: meta };
    if (params !== null && typeof params === "object" && !Array.isArray(params)) {
      return { ...(params as Record<string, unknown>), _meta: meta } as P;
    }
    return params as P;
  }

  async function call<R, P>(
    method: string,
    params?: P,
    callOptions: RpcCallOptions = {},
  ): Promise<R> {
    if (closed) throw new RpcTransportError("client closed");
    const id = String(nextId++);
    const req: RpcRequest = {
      jsonrpc: JSONRPC_VERSION,
      id,
      method,
      ...(() => {
        const withMeta = paramsWithMeta(params);
        return withMeta !== undefined ? { params: withMeta } : {};
      })(),
    };

    return new Promise<R>((resolve, reject) => {
      const { signal } = callOptions;
      // Aborting the transport request propagates cancellation through the
      // server request context; no second cancellation protocol is needed.
      const onAbort = () => {
        if (!pending.has(id)) return;
        pending.delete(id);
        reject(new RpcTransportError("aborted"));
      };
      // Detach the abort listener once the request settles by any path —
      // otherwise a long-lived / shared signal accumulates one dead
      // listener per completed call ({ once: true } only fires on abort).
      const detach = () => signal?.removeEventListener("abort", onAbort);
      pending.set(id, {
        resolve: (value) => {
          detach();
          (resolve as (v: unknown) => void)(value);
        },
        reject: (err) => {
          detach();
          reject(err);
        },
      });

      if (signal) {
        if (signal.aborted) {
          onAbort();
          return;
        }
        signal.addEventListener("abort", onAbort, { once: true });
      }

      transport.send(req, signal, { idempotencyKey: callOptions.idempotencyKey }).catch((err) => {
        if (!pending.has(id)) return; // already aborted/settled
        pending.delete(id);
        detach();
        reject(err);
      });
    });
  }

  async function notify<P>(method: string, params?: P): Promise<void> {
    if (closed) throw new RpcTransportError("client closed");
    const withMeta = paramsWithMeta(params);
    const msg: RpcNotification = {
      jsonrpc: JSONRPC_VERSION,
      method,
      ...(withMeta !== undefined ? { params: withMeta } : {}),
    };
    await transport.send(msg);
  }

  function subscribe(method: string, handler: NotificationHandler): () => void {
    let set = subscribers.get(method);
    if (!set) {
      set = new Set();
      subscribers.set(method, set);
    }
    set.add(handler);
    return () => {
      const current = subscribers.get(method);
      if (!current) return;
      current.delete(handler);
      if (current.size === 0) subscribers.delete(method);
    };
  }

  async function close(): Promise<void> {
    if (closed) return;
    closed = true;
    failAllPending(new RpcTransportError("client closed"));
    subscribers.clear();
    await transport.close();
  }

  return { call, notify, subscribe, close };
}
