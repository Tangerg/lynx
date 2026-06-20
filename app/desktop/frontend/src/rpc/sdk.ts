// Lyra Runtime Protocol SDK — the one ergonomic entry point.
//
// The protocol is transport-agnostic (docs/protocol/TRANSPORT.md): the same JSON-RPC
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
import { isErrorType } from "./errors";
import { createMethods, type Methods } from "./methods";
import type { Transport } from "./transport";

/** Options for [createLyraClient]. */
export interface LyraClientOptions {
  /**
   * Auto-recovery hook. When a `call` fails with `capability_not_negotiated`
   * (the backend has no negotiated session — it restarted, or never handshook),
   * this is invoked to re-run `runtime.initialize` over the raw rpc, then the
   * call is retried ONCE. Omitted ⇒ no auto-recovery (a failed handshake just
   * surfaces). The app injects [main/handshake.performHandshake].
   */
  reinit?: (rpc: RpcClient) => Promise<void>;
}

// withReinit decorates `call` so a lost-session error self-heals: re-handshake
// then retry once. `runtime.initialize` is excluded (it's the recovery itself —
// retrying it would loop) and the retry is single (a still-failing call after a
// fresh handshake is a real error, not a stale session). All other surface —
// notify / subscribe / close — passes through untouched. Exported for tests.
export function withReinit(rpc: RpcClient, reinit: (rpc: RpcClient) => Promise<void>): RpcClient {
  return {
    ...rpc,
    call: async <R, P>(method: string, params?: P, signal?: AbortSignal): Promise<R> => {
      try {
        return await rpc.call<R, P>(method, params, signal);
      } catch (err) {
        if (method === "runtime.initialize" || !isErrorType(err, "capability_not_negotiated")) {
          throw err;
        }
        await reinit(rpc);
        return rpc.call<R, P>(method, params, signal);
      }
    },
  };
}

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
export function createLyraClient(transport: Transport, opts?: LyraClientOptions): LyraClient {
  const rpc = createRpcClient(transport);
  // The decorated rpc is what the typed methods (and the exposed `rpc` handle)
  // call through, so auto-recovery covers everything; close() still targets the
  // real client so teardown tears down the transport.
  const client = opts?.reinit ? withReinit(rpc, opts.reinit) : rpc;
  return Object.assign(createMethods(client), {
    rpc: client,
    close: () => rpc.close(),
  });
}
