// Lyra Runtime Protocol SDK — the one ergonomic entry point.
//
// The protocol is transport-agnostic (docs/protocol/TRANSPORT.md): the same JSON-RPC
// semantics ride InProcess / HTTP. So the SDK takes a `Transport` and
// nothing else — inject the transport, get back a fully-typed client:
//
//   const client = createLyraClient(createHttpTransport({ baseUrl }));
//   await client.runtime.discover();
//   const { result, events } = await client.runs.start({ ... });
//   for await (const ev of events) reduce(ev.event);
//   await client.close();
//
// `LyraClient` is the complete typed method surface
// (client.sessions.list(), …) plus `close()` for teardown.
//
// Transport construction (HTTP / in-memory) stays separate —
// see transports/*. Sidecar metadata (/v2/info, /v2/health/{live,ready}) is an
// HTTP-transport-only concern and lives in sidecar.ts, not here.

import { createRpcClient } from "./client";
import { createMethods, type Methods } from "./methods";
import type { MutationJournal } from "./mutationJournal";
import type { RequestMeta, ServerCapabilities } from "./wire.generated";
import type { Transport } from "./transport";

/** Options for [createLyraClient]. */
export interface LyraClientOptions {
  requestMeta?: () => RequestMeta | undefined;
  /**
   * What the server said it can do, or null before discovery. Supplying it lets the
   * capability preflight refuse a gated call locally instead of round-tripping to
   * learn what the negotiation already said.
   */
  capabilities?: () => ServerCapabilities | null | undefined;
  /** Durable command identity owner supplied by the embedding application. */
  mutationJournal?: MutationJournal;
}

export interface LyraClient extends Methods {
  /** Tear down the client + the underlying transport. */
  close(): Promise<void>;
}

/** Build a Lyra Runtime Protocol client over the given transport. */
export function createLyraClient(transport: Transport, opts?: LyraClientOptions): LyraClient {
  const rpc = createRpcClient(transport, { requestMeta: opts?.requestMeta });
  return Object.assign(
    createMethods(rpc, {
      capabilities: opts?.capabilities,
      requestMeta: opts?.requestMeta,
      mutationJournal: opts?.mutationJournal,
    }),
    {
      close: async () => {
        opts?.mutationJournal?.dispose();
        await rpc.close();
      },
    },
  );
}
