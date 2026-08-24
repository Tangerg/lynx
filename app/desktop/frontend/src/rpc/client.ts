// Typed JSON-RPC client wrapping a Transport. Owns id allocation,
// response correlation, notification dispatch. See app/runtime/doc/API.md §1.
//
// Correlation pattern borrowed from kimi-code's controlledPromise idea:
// each Request creates a pending entry with resolve/reject handles;
// the recv() loop pops the entry by id and settles the promise.
// Notifications go through subscribe() — no id, no waiter.

import { errorMessage, RpcError, RpcProtocolError, RpcTransportError } from "./errors";
import {
  NOTIFICATIONS_RUN_EVENT,
  runEventReliability,
  type ProblemData,
  type RequestMeta,
} from "@lyra/runtime-contract/wire";
import type {
  Transport,
  TransportEvent,
  TransportRequest,
  TransportResponseMetadata,
} from "./transport";
import {
  wireMethodAcceptsReplayCursor,
  wireMethodRequiresIdempotency,
  wireMethodReturnsValue,
  type WireMethodName,
  type WireParams,
  type WireResult,
} from "@lyra/runtime-contract/methods";
import {
  isWireNotificationName,
  validateMethodResult,
  validateNotificationParams,
  validateWire,
  type WireNotificationName,
  type WireNotificationParams,
} from "@lyra/runtime-contract/validate";
import type { RpcId, RpcMessage } from "./types";
import { JSONRPC_VERSION, isErrorResponse, isNotification, isResponse } from "./types";

export interface NotificationObserver<M extends WireNotificationName = WireNotificationName> {
  next(params: WireNotificationParams[M], requestRpcId: RpcId): void;
  /** A notification-local protocol error names its response stream; a connection
   *  failure has no request owner and terminates every observer. */
  error(error: RpcProtocolError | RpcTransportError, requestRpcId?: RpcId): void;
}

export type StreamEndHandler = (event: Extract<TransportEvent, { type: "streamEnd" }>) => void;

function notificationEventType(params: unknown): unknown {
  if (params === null || typeof params !== "object" || Array.isArray(params)) return undefined;
  const event = (params as Record<string, unknown>).event;
  if (event === null || typeof event !== "object" || Array.isArray(event)) return undefined;
  return (event as Record<string, unknown>).type;
}

export interface RpcClientOptions {
  requestMeta?: () => RequestMeta | undefined;
}

export interface RpcCallOptions {
  signal?: AbortSignal;
  idempotencyKey?: string;
  /** Expected durable replay store. Runtime refuses the key before business
   * admission if this no longer matches discovery. */
  idempotencyNamespace?: string;
  /** Resume a stream from the last event this client folded (§5.5). The runtime
   *  replays from just after it, or refuses when the cursor is not addressable —
   *  which is the caller's signal to rebuild from a cold read instead. */
  lastEventId?: string;
  /**
   * Metadata snapshot selected by a typed call. This keeps capability preflight
   * and the emitted request on the same client declaration even when the
   * configured metadata provider is dynamic.
   */
  requestMeta?: RequestMeta | null;
  /** Bind a stream owner before Transport.send can deliver its first frame. */
  onRequestRpcId?: (id: RpcId) => void;
}

export interface RpcClient {
  /** Send a Request and resolve with its result, or reject with RpcError. */
  call<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    options?: RpcCallOptions,
  ): Promise<WireResult<M>>;
  /** Subscribe to inbound notifications matching `method`. Returns an unsubscribe fn. */
  subscribe<M extends WireNotificationName>(
    method: M,
    observer: NotificationObserver<M>,
  ): () => void;
  /** Observe transport-level termination of one streaming response. */
  onStreamEnd(handler: StreamEndHandler): () => void;
  /** Tear down the client + underlying transport. */
  close(): Promise<void>;
}

interface Pending {
  method: WireMethodName;
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
  const subscribers = new Map<WireNotificationName, Set<NotificationObserver>>();
  const streamEndHandlers = new Set<StreamEndHandler>();
  let closed = false;
  let closePromise: Promise<void> | undefined;

  function failAllPending(failure: RpcTransportError): void {
    for (const { reject } of pending.values()) reject(failure);
    pending.clear();
  }

  function failConnection(failure: RpcTransportError): void {
    closed = true;
    failAllPending(failure);
    for (const observers of subscribers.values()) {
      for (const observer of observers) observer.error(failure);
    }
    subscribers.clear();
    streamEndHandlers.clear();
  }

  // Long-running pump that drains the transport's recv() into pending
  // promises + subscribers. When the stream ends — whether it throws or
  // closes cleanly — no further Responses can arrive, so every in-flight
  // request must be settled (rejected). Handling only the throw path left
  // pending calls hung forever on a clean EOS (for example, an InProcess
  // transport whose recv() ends without an exception).
  const receiveLoop = (async () => {
    try {
      for await (const event of transport.recv()) {
        dispatchInbound(event);
      }
      failConnection(new RpcTransportError("transport stream ended"));
    } catch (err) {
      failConnection(new RpcTransportError(`transport recv() failed: ${errorMessage(err)}`));
    }
  })();

  function dispatchInbound(event: TransportEvent): void {
    if (event.type === "requestError") {
      const entry = pending.get(event.rpcId);
      if (!entry) return;
      pending.delete(event.rpcId);
      entry.reject(event.error);
      return;
    }
    if (event.type === "streamEnd") {
      for (const handler of streamEndHandlers) handler(event);
      return;
    }
    if (isResponse(event.message) && event.message.id !== event.requestRpcId) {
      // A Transport merges many concurrent response bodies into one receive
      // channel. Its source request is authoritative: trusting only the envelope
      // id would let a malformed frame from request A settle request B and strand A.
      const entry = pending.get(event.requestRpcId);
      if (entry) {
        pending.delete(event.requestRpcId);
        entry.reject(
          new RpcProtocolError(
            `${entry.method} response`,
            [
              {
                path: `${entry.method}.response.id`,
                detail: `must match request id ${event.requestRpcId}`,
              },
            ],
            event.metadata?.requestId,
          ),
        );
      }
      return;
    }
    dispatchMessage(event.message, event.requestRpcId, event.metadata);
  }

  function dispatchMessage(
    msg: RpcMessage,
    requestRpcId: RpcId,
    metadata?: TransportResponseMetadata,
  ): void {
    if (isResponse(msg)) {
      const entry = pending.get(msg.id);
      if (!entry) return; // unsolicited or already settled — drop silently
      pending.delete(msg.id);
      if (isErrorResponse(msg)) {
        const payload = msg.error;
        if (payload.data !== undefined) {
          const violations = validateWire("ProblemData", payload.data);
          if (violations.length > 0) {
            entry.reject(
              new RpcProtocolError(`${entry.method} error data`, violations, metadata?.requestId),
            );
            return;
          }
        }
        // The generated validator above is the trust boundary that turns the raw
        // envelope payload into the generated discriminated union.
        entry.reject(
          new RpcError(
            {
              code: payload.code,
              message: payload.message,
              data: payload.data as ProblemData | undefined,
            },
            metadata?.requestId,
          ),
        );
      } else {
        const result = msg.result;
        const violations = validateMethodResult(entry.method, result);
        if (violations.length > 0) {
          entry.reject(
            new RpcProtocolError(`${entry.method} result`, violations, metadata?.requestId),
          );
          return;
        }
        entry.resolve(wireMethodReturnsValue(entry.method) ? result : undefined);
      }
      return;
    }
    if (isNotification(msg)) {
      if (!isWireNotificationName(msg.method)) return;
      const handlers = subscribers.get(msg.method);
      if (!handlers) return;
      const violations = validateNotificationParams(msg.method, msg.params);
      if (violations.length > 0) {
        const failure = new RpcProtocolError(
          `${msg.method} params`,
          violations,
          metadata?.requestId,
        );
        if (
          msg.method === NOTIFICATIONS_RUN_EVENT &&
          runEventReliability(notificationEventType(msg.params)) === "ephemeral"
        ) {
          console.warn(`[rpc] dropping invalid ephemeral notification: ${failure.message}`);
          return;
        }
        for (const observer of handlers) observer.error(failure, requestRpcId);
        return;
      }
      for (const observer of handlers) {
        try {
          observer.next(msg.params as WireNotificationParams[typeof msg.method], requestRpcId);
        } catch (err) {
          // Subscribers must not crash the dispatch loop. Log and move on.
          console.error(`[rpc] notification handler for "${msg.method}" threw:`, err);
        }
      }
      return;
    }
    // Unexpected: server-initiated Requests are not in our protocol.
    // Drop them — see app/runtime/doc/API.md §1.1 (we don't do server→client RPC).
    console.warn("[rpc] dropping unexpected server-initiated Request", msg);
  }

  function paramsWithMeta<P>(
    params: P | undefined,
    meta: RequestMeta | null | undefined = options.requestMeta?.(),
  ): unknown {
    if (!meta) return params;
    if (params === undefined) return { _meta: meta };
    if (params !== null && typeof params === "object" && !Array.isArray(params)) {
      return Object.assign({}, params, { _meta: meta });
    }
    return params;
  }

  async function call<M extends WireMethodName>(
    method: M,
    params: WireParams<M>,
    callOptions: RpcCallOptions = {},
  ): Promise<WireResult<M>> {
    if (closed) throw new RpcTransportError("client closed");
    const requiresIdempotency = wireMethodRequiresIdempotency(method);
    if (callOptions.idempotencyKey !== undefined && !requiresIdempotency) {
      throw new TypeError(`${method} does not accept an idempotency key`);
    }
    if (
      callOptions.idempotencyNamespace !== undefined &&
      callOptions.idempotencyKey === undefined
    ) {
      throw new TypeError("An idempotency namespace requires an idempotency key");
    }
    if (callOptions.lastEventId !== undefined) {
      if (!wireMethodAcceptsReplayCursor(method)) {
        throw new TypeError(`${method} does not accept a run replay cursor`);
      }
      if (requiresIdempotency && callOptions.idempotencyKey === undefined) {
        throw new TypeError("A run command replay cursor requires an idempotency key");
      }
    }
    const id = String(nextId++);
    callOptions.onRequestRpcId?.(id);
    const req: TransportRequest = {
      jsonrpc: JSONRPC_VERSION,
      id,
      method,
      ...(() => {
        const withMeta = paramsWithMeta(params, callOptions.requestMeta);
        return withMeta !== undefined ? { params: withMeta } : {};
      })(),
    };

    return new Promise<WireResult<M>>((resolve, reject) => {
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
        method,
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

      transport
        .send(req, signal, {
          idempotencyKey: callOptions.idempotencyKey,
          idempotencyNamespace: callOptions.idempotencyNamespace,
          lastEventId: callOptions.lastEventId,
        })
        .catch((err) => {
          if (!pending.has(id)) return; // already aborted/settled
          pending.delete(id);
          detach();
          reject(err);
        });
    });
  }

  function subscribe<M extends WireNotificationName>(
    method: M,
    observer: NotificationObserver<M>,
  ): () => void {
    // The map is heterogeneous by method. Erasure happens once, here; dispatch only
    // invokes the observer after the generated validator for this same key succeeds.
    const validatedObserver = observer as NotificationObserver;
    let set = subscribers.get(method);
    if (!set) {
      set = new Set();
      subscribers.set(method, set);
    }
    set.add(validatedObserver);
    return () => {
      const current = subscribers.get(method);
      if (!current) return;
      current.delete(validatedObserver);
      if (current.size === 0) subscribers.delete(method);
    };
  }

  function onStreamEnd(handler: StreamEndHandler): () => void {
    streamEndHandlers.add(handler);
    return () => streamEndHandlers.delete(handler);
  }

  function close(): Promise<void> {
    closePromise ??= (async () => {
      // `closed` is the request-admission / correlation state and can already
      // be true because recv() ended. Transport ownership is independent: the
      // public close contract must still run and join its one teardown.
      if (!closed) failConnection(new RpcTransportError("client closed"));
      let closeFailure: unknown;
      try {
        await transport.close();
      } catch (error) {
        closeFailure = error;
      }
      // The receive pump is created by RpcClient, not by Transport. Transport
      // close makes recv() terminal; joining the consumer here guarantees that
      // close() owns the complete lifecycle and no final iterator continuation
      // escapes into the next client/test generation.
      await receiveLoop;
      if (closeFailure !== undefined) throw closeFailure;
    })();
    return closePromise;
  }

  return { call, subscribe, onStreamEnd, close };
}
