// Transport interface — bidirectional pipe for JSON-RPC messages.
// See docs/TRANSPORT.md §2. Implementations: transports/http.ts (Web /
// future facade), transports/memory.ts (tests). Future: Wails IPC when
// the packaged-web client materialises.
//
// Send() is fire-and-forget: it returns when the message has been
// handed off, not when the peer has processed it. Recv() returns a
// channel-like AsyncIterable that yields inbound messages until close.
// Response correlation by `id` is the RpcClient's job, not Transport's.

import type { RpcMessage } from "./types";

export interface Transport {
  /** Queue an outbound message. */
  send(msg: RpcMessage, signal?: AbortSignal): Promise<void>;
  /**
   * Stream of inbound messages. Yields until the transport disconnects,
   * after which the iterator returns. Multiple readers are not supported
   * — RpcClient is the sole consumer.
   */
  recv(): AsyncIterable<RpcMessage>;
  /** Tear down the transport — abort any pending send, close recv stream. */
  close(): Promise<void>;
}
