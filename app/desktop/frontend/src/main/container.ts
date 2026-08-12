// Composition root — owns the app's Runtime Protocol client and Wails host.
// Singleton instead of Context because non-component code (zustand effects,
// plugin setup) calls these too; tests inject fakes via `setContainer()`.

import { runtimeRequestMeta } from "@/main/runtimeProtocol";
import { negotiatedCapabilities } from "@/plugins/builtin/runtime/public/capabilities";
import { currentRuntimeEndpoint } from "@/plugins/builtin/runtime/public/endpoint";
import type { DesktopBootstrap, DesktopHostClient, LyraClient, SidecarClient } from "@/rpc";
import {
  createDesktopHostClient,
  createHttpTransport,
  createLyraClient,
  createSidecarClient,
} from "@/rpc";

export interface Container {
  /**
   * Shared, lazily-constructed Lyra Runtime Protocol SDK client for app use.
   * Builds the transport lazily and caches one client per active endpoint and
   * local-token signature. Runtime configuration is restored before discovery;
   * changing it produces a new client instead of leaving callers pinned to the
   * startup default. Tests can override with `setContainer({ client })`.
   */
  client: () => LyraClient;
  /** Typed HTTP operational endpoints owned by the Runtime transport adapter. */
  sidecar: () => SidecarClient;
  /** App-owned Wails capability boundary. It never becomes Runtime Protocol. */
  desktop: DesktopHostClient;
}

let desktopBootstrap: DesktopBootstrap | null = null;

function localTokenFor(endpoint: string): string | undefined {
  const local = desktopBootstrap?.localRuntime;
  if (!local) return undefined;
  const normalized = endpoint.replace(/\/+$/, "");
  return normalized === local.endpoint.replace(/\/+$/, "") ? local.localToken : undefined;
}

function defaultContainer(): Container {
  let shared: { signature: string; client: LyraClient } | null = null;
  let sidecar: { endpoint: string; client: SidecarClient } | null = null;
  return {
    client: () => {
      const baseUrl = currentRuntimeEndpoint();
      const localToken = localTokenFor(baseUrl);
      const signature = `${baseUrl}\u0000${localToken ?? ""}`;
      if (shared?.signature === signature) return shared.client;
      const client = createLyraClient(createHttpTransport({ baseUrl, localToken }), {
        requestMeta: runtimeRequestMeta,
        capabilities: negotiatedCapabilities,
      });
      shared = { signature, client };
      return client;
    },
    sidecar: () => {
      const endpoint = currentRuntimeEndpoint();
      if (sidecar?.endpoint === endpoint) return sidecar.client;
      const client = createSidecarClient({ baseUrl: endpoint });
      sidecar = { endpoint, client };
      return client;
    },
    desktop: createDesktopHostClient(),
  };
}

let instance: Container = defaultContainer();

export function getContainer(): Container {
  return instance;
}

/** Test seam — swap any subset of gateways with fakes. Other slots stay
 *  on the current defaults. */
export function setContainer(next: Partial<Container>): void {
  if (next.desktop) desktopBootstrap = null;
  instance = { ...instance, ...next };
}

/** Load app-owned bootstrap data before any plugin can construct an RPC client. */
export async function initializeDesktopHost(): Promise<void> {
  desktopBootstrap = await instance.desktop.bootstrap();
}

/** Test seam — restore every gateway to its default wiring. Call from
 *  `afterEach` so one test's stubs don't bleed into the next. */
export function resetContainer(): void {
  desktopBootstrap = null;
  instance = defaultContainer();
}
