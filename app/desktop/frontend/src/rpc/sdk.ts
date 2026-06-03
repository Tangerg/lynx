// Lyra Runtime Protocol SDK — the one ergonomic entry point.
//
// The protocol is transport-agnostic (docs/TRANSPORT.md): the same JSON-RPC
// semantics ride InProcess / IPC / HTTP. So the SDK takes a `Transport` and
// nothing else — inject the transport, get back a fully-typed client:
//
//   const client = createLyraClient(createHttpTransport({ baseUrl }));
//   await client.runtime.initialize({ ... });
//   const { result, events } = await client.runs.start({ ... });
//   for await (const ev of events) reduce(ev.event);
//   await client.close();
//
// `LyraClient` is the typed method surface (client.sessions.list(), …) plus
// the low-level `rpc` handle (raw call/notify/subscribe for anything the
// typed surface doesn't wrap yet) and `close()` for teardown.
//
// Transport construction (HTTP / in-memory / future IPC) stays separate —
// see transports/*. Sidecar metadata (/v2/info, /v2/health) is an
// HTTP-transport-only concern and lives in sidecar.ts, not here.

import { createRpcClient, type RpcClient } from "./client";
import { createMethods, type Methods } from "./methods";
import type { Transport } from "./transport";

export interface LyraClient extends Methods {
  /**
   * The low-level JSON-RPC client — raw `call` / `notify` / `subscribe` for
   * advanced use (e.g. listening to a notification method the typed surface
   * doesn't wrap, or issuing a method added server-side ahead of the SDK).
   */
  readonly rpc: RpcClient;
  /** Tear down the client + the underlying transport. */
  close(): Promise<void>;
}

/** Build a Lyra Runtime Protocol client over the given transport. */
export function createLyraClient(transport: Transport): LyraClient {
  const rpc = createRpcClient(transport);
  return Object.assign(createMethods(rpc), {
    rpc,
    close: () => rpc.close(),
  });
}
