// Transport interface — bidirectional pipe for JSON-RPC messages.
// See docs/protocol/TRANSPORT.md §2. Implementations: transports/http.ts (Web /
// future facade), transports/memory.ts (tests). Future: Wails IPC when
// the packaged-web client materialises.
//
// Send() is fire-and-forget: it returns when the message has been
// handed off, not when the peer has processed it. Recv() returns a
// channel-like AsyncIterable that yields inbound messages until close.
// Response correlation by `id` is the RpcClient's job, not Transport's.

import type { RpcMessage } from "./types";

/**
 * CLIENT-SYNTHETIC notification a transport injects into its own inbound
 * channel when a streaming response dies abnormally (network drop, runtime
 * restart — anything that ends the stream other than a caller abort). It is
 * never sent by the server and never goes on the wire; it exists so the run
 * stream layer (rpc/stream.ts) can close the affected runs' channels instead
 * of leaving their consumers awaiting forever. `runIds` = the run ids whose
 * events were observed on the dead stream.
 */
export const STREAM_DOWN_METHOD = "transport.streamDown";

/** The one non-run streaming method (AUX_API §3.1). Named here — next to the
 *  STREAM_DOWN synthetic that special-cases it — so the transport, the stream
 *  layer, and the subscriber plugin all share one literal. */
export const WORKSPACE_SUBSCRIBE_METHOD = "workspace.subscribe";

export interface StreamDownParams {
  runIds: string[];
  /** The streaming method whose POST stream ended — set for non-run streams
   *  (workspace.subscribe), which carry no runId to attribute. For those the
   *  synthetic fires on ANY non-abort end (even a graceful EOS): the
   *  subscription is connection-scoped, so stream end = subscription over and
   *  the consumer must resubscribe (AUX_API §3.1). */
  method?: string;
}

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
