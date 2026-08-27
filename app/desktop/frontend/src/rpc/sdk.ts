// ScopeApp Runtime Protocol SDK — the one ergonomic entry point.
//
// The protocol is transport-agnostic (app/runtime/doc/TRANSPORT.md): the same JSON-RPC
// semantics ride InProcess / HTTP. So the SDK takes a `Transport` and
// nothing else — inject the transport, get back a fully-typed client:
//
//   const client = createScopeAppClient(createHttpTransport({ baseUrl }));
//   await client.runtime.discover();
//   const { result, events } = await client.runs.start({ ... });
//   for await (const ev of events) reduce(ev.event);
//   await client.close();
//
// `ScopeAppClient` is the complete typed method surface
// (client.sessions.list(), …) plus `close()` for teardown.
//
// Transport construction (HTTP / in-memory) stays separate —
// see transports/*. Sidecar metadata (/v2/info, /v2/health/{live,ready}) is an
// HTTP-transport-only concern and lives in sidecar.ts, not here.

import { createRpcClient } from "./client";
import { createMethods, type Methods } from "./methods";
import type { MutationJournal } from "./mutationJournal";
import type { RequestMeta, ServerCapabilities } from "@scopeapp/runtime-contract/wire";
import type { Transport } from "./transport";

/** Options for [createScopeAppClient]. */
export interface ScopeAppClientOptions {
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

export interface ScopeAppClient extends Methods {
  /** Tear down the client + the underlying transport. */
  close(): Promise<void>;
}

/** Build a ScopeApp Runtime Protocol client over the given transport. */
export function createScopeAppClient(
  transport: Transport,
  opts?: ScopeAppClientOptions,
): ScopeAppClient {
  const rpc = createRpcClient(transport, { requestMeta: opts?.requestMeta });
  let closePromise: Promise<void> | undefined;
  const close = (): Promise<void> => {
    closePromise ??= (async () => {
      let journalFailure: unknown;
      try {
        opts?.mutationJournal?.dispose();
      } catch (error) {
        journalFailure = error;
      }
      let transportFailure: unknown;
      try {
        await rpc.close();
      } catch (error) {
        transportFailure = error;
      }
      if (journalFailure !== undefined && transportFailure !== undefined) {
        throw new AggregateError(
          [journalFailure, transportFailure],
          "Runtime client ownership and transport cleanup both failed",
        );
      }
      if (journalFailure !== undefined) throw journalFailure;
      if (transportFailure !== undefined) throw transportFailure;
    })();
    return closePromise;
  };
  return Object.assign(
    createMethods(rpc, {
      capabilities: opts?.capabilities,
      requestMeta: opts?.requestMeta,
      mutationJournal: opts?.mutationJournal,
    }),
    { close },
  );
}
