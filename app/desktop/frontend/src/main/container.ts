// Composition root — owns the app's Runtime Protocol entry points: the SDK
// client (createLyraClient over an HTTP transport) + the sidecar probe.
// Singleton instead of Context because non-component code (zustand effects,
// plugin setup) calls these too; tests inject fakes via `setContainer()`.

import { PROTOCOL_VERSION, RUNTIME_BASE } from "@/main/config";
import { performHandshake } from "@/main/handshake";
import { getConfig } from "@/plugins/sdk/config";
import type { LyraClient, ShellClient, SidecarClient } from "@/rpc";
import {
  createHttpTransport,
  createLyraClient,
  createShellClient,
  createSidecarClient,
} from "@/rpc";

export interface Container {
  /**
   * Shared, lazily-constructed Lyra Runtime Protocol SDK client for app use.
   * Builds the transport (one SSE connection) on first call and caches it for
   * the life of the container — the single entry point so callers don't each
   * spin up their own. Tests get a fresh cache via `resetContainer()` (and can
   * override with `setContainer({ client })`).
   */
  client: () => LyraClient;
  /**
   * Sidecar HTTP probe — pre-instantiated because it doesn't open a
   * persistent connection (each call is a one-shot fetch). HTTP-transport
   * only; safe to call against a backend that doesn't implement it yet (the
   * caller handles the RpcTransportError).
   */
  sidecar: SidecarClient;
  /**
   * Wails shell asset client (sideloaded-plugin manifest). NOT the Runtime
   * Protocol — host/packaging metadata served by the desktop shell. Routed
   * through the container so sideload discovery is injectable in tests and the
   * "single outbound seam" invariant holds (no bare fetch in plugins/host).
   */
  shell: ShellClient;
}

function defaultContainer(): Container {
  const baseUrl = RUNTIME_BASE;
  let shared: LyraClient | null = null;
  return {
    // Read `api.localToken` at build time so plugins (e.g. a Wails-side
    // bootstrap reading `~/.lyra/local-token`) can set it via
    // `host.config.set("api.localToken", ...)` before the first client is
    // built. Local-loopback process gate (TRANSPORT.md §11); dev mock leaves
    // it unset.
    client: () =>
      (shared ??= createLyraClient(
        createHttpTransport({
          baseUrl,
          localToken: getConfig<string>("api.localToken") ?? undefined,
          protocolVersion: PROTOCOL_VERSION,
        }),
        // Auto-recover a lost session: if a call hits capability_not_negotiated
        // (backend restarted / never handshook), re-run runtime.initialize and
        // retry. This is what makes a backend restart transparent — the
        // workspace-events resubscribe loop and any business call self-heal.
        { reinit: performHandshake },
      )),
    sidecar: createSidecarClient({ baseUrl }),
    shell: createShellClient({ baseUrl }),
  };
}

let instance: Container = defaultContainer();

export function getContainer(): Container {
  return instance;
}

/** Test seam — swap any subset of gateways with fakes. Other slots stay
 *  on the current defaults. */
export function setContainer(next: Partial<Container>): void {
  instance = { ...instance, ...next };
}

/** Test seam — restore every gateway to its default wiring. Call from
 *  `afterEach` so one test's stubs don't bleed into the next. */
export function resetContainer(): void {
  instance = defaultContainer();
}
