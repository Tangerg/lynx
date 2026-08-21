// Transport interface — request output plus typed inbound transport events.
// See app/runtime/doc/TRANSPORT.md §2. Implementations: transports/http.ts (Web /
// future facade), transports/memory.ts (tests).
//
// Send() is fire-and-forget: it returns when the message has been
// handed off, not when the peer has processed it. Recv() returns a
// channel-like AsyncIterable that yields inbound messages and transport lifecycle
// events until close.
// Response correlation by `id` is the RpcClient's job, not Transport's.

import type { RpcId, RpcMessage, RpcRequest } from "./types";
import type { WireMethodName, WireStreamingMethodName } from "@lyra/runtime-contract/methods";

/** The one non-run streaming method, shared by transport and stream lifecycle. */
export const RUNTIME_SUBSCRIBE_METHOD = "runtime.subscribe" satisfies WireMethodName;

export type TransportRequest = Omit<RpcRequest, "method"> & { method: WireMethodName };

/** Typed response metadata carried outside the JSON-RPC envelope. */
export interface TransportResponseMetadata {
  /** Runtime-generated HTTP Request-Id used to correlate logs and traces. */
  requestId?: string;
}

export type TransportEvent =
  | {
      type: "message";
      message: RpcMessage;
      /** Outbound JSON-RPC request whose response stream carried this frame. */
      requestRpcId: RpcId;
      metadata?: TransportResponseMetadata;
    }
  | { type: "requestError"; rpcId: RpcId; error: Error }
  | {
      type: "streamEnd";
      method: WireStreamingMethodName;
      /** Outbound JSON-RPC request that owned the ended response stream. */
      requestRpcId: RpcId;
      error?: Error;
      metadata?: TransportResponseMetadata;
    };

export interface Transport {
  /** Queue an outbound request. Client notifications are not in this protocol. */
  send(msg: TransportRequest, signal?: AbortSignal, options?: TransportSendOptions): Promise<void>;
  /**
   * Stream of inbound messages and lifecycle events. Yields until the transport disconnects,
   * after which the iterator returns. Multiple readers are not supported
   * — RpcClient is the sole consumer.
   */
  recv(): AsyncIterable<TransportEvent>;
  /** Tear down the transport — abort any pending send, close recv stream. */
  close(): Promise<void>;
}

export interface TransportSendOptions {
  idempotencyKey?: string;
  /** Opaque durable replay-store identity, carried as
   * `Idempotency-Namespace` beside the key. */
  idempotencyNamespace?: string;
  /** Stream resume cursor, carried as `Last-Event-Id` (TRANSPORT §9.2). Transport
   *  metadata, never request params. */
  lastEventId?: string;
}
