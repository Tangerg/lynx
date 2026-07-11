// Composition root — owns the app's Runtime Protocol client and the local
// desktop-shell asset client.
// Singleton instead of Context because non-component code (zustand effects,
// plugin setup) calls these too; tests inject fakes via `setContainer()`.

import { RUNTIME_BASE, RUNTIME_ENDPOINT_CONFIG_KEY } from "@/main/config";
import { runtimeRequestMeta } from "@/main/runtimeProtocol";
import { getConfig } from "@/plugins/sdk/config";
import type { LyraClient, ShellClient } from "@/rpc";
import { createHttpTransport, createLyraClient, createShellClient } from "@/rpc";

export interface Container {
  /**
   * Shared, lazily-constructed Lyra Runtime Protocol SDK client for app use.
   * Builds the transport lazily and caches one client per active endpoint and
   * local-token signature. Runtime configuration is restored before discovery;
   * changing it produces a new client instead of leaving callers pinned to the
   * startup default. Tests can override with `setContainer({ client })`.
   */
  client: () => LyraClient;
  /**
   * Wails shell asset client (sideloaded-plugin manifest). NOT the Runtime
   * Protocol — host/packaging metadata served by the desktop shell. Routed
   * through the container so sideload discovery is injectable in tests and the
   * "single outbound seam" invariant holds (no bare fetch in plugins/host).
   */
  shell: ShellClient;
}

function defaultContainer(): Container {
  let shared: { signature: string; client: LyraClient } | null = null;
  return {
    client: () => {
      const baseUrl = getConfig<string>(RUNTIME_ENDPOINT_CONFIG_KEY) ?? RUNTIME_BASE;
      const localToken = getConfig<string>("api.localToken") ?? undefined;
      const signature = `${baseUrl}\u0000${localToken ?? ""}`;
      if (shared?.signature === signature) return shared.client;
      const client = createLyraClient(createHttpTransport({ baseUrl, localToken }), {
        requestMeta: runtimeRequestMeta,
      });
      shared = { signature, client };
      return client;
    },
    // Shell assets belong to the local desktop process, not to the selectable
    // Runtime Protocol endpoint.
    shell: createShellClient({ baseUrl: RUNTIME_BASE }),
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
