// Composition root — owns the app's Runtime Protocol client and Wails host.
// Singleton instead of Context because non-component code (zustand effects,
// plugin setup) calls these too; tests inject fakes via `setContainer()`.

import { runtimeRequestMeta } from "@/main/runtimeProtocol";
import { negotiatedCapabilities } from "@/plugins/builtin/runtime/public/capabilities";
import { currentRuntimeEndpoint } from "@/plugins/builtin/runtime/public/endpoint";
import { installedRuntimeMutationJournalStorage } from "@/plugins/builtin/runtime/public/mutationJournal";
import type { DesktopBootstrap, DesktopHostClient, LyraClient, SidecarClient } from "@/rpc";
import type { RuntimeMutationJournalStorage } from "@/plugins/builtin/runtime/public/mutationJournal";
import {
  createDesktopHostClient,
  createHttpTransport,
  createLyraClient,
  createMutationJournal,
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

interface DefaultContainerOwner {
  readonly container: Container;
  initializeDesktopHost(desktop: DesktopHostClient): Promise<void>;
  replaceDesktopHost(): void;
  dispose(): Promise<void>;
}

function defaultContainer(): DefaultContainerOwner {
  let shared: {
    signature: string;
    storage: RuntimeMutationJournalStorage | null;
    client: LyraClient;
  } | null = null;
  let sidecar: { endpoint: string; client: SidecarClient } | null = null;
  const retiring = new Set<Promise<void>>();
  let desktopBootstrap: DesktopBootstrap | null = null;
  let bootstrapGeneration = 0;
  let closed = false;
  let disposal: Promise<void> | undefined;
  const assertOpen = () => {
    if (closed) throw new Error("Desktop container is closed");
  };
  const retire = (client: LyraClient) => {
    let closing!: Promise<void>;
    closing = client
      .close()
      .catch(() => undefined)
      .finally(() => retiring.delete(closing));
    retiring.add(closing);
  };
  const localTokenFor = (endpoint: string): string | undefined => {
    const local = desktopBootstrap?.localRuntime;
    if (!local) return undefined;
    const normalized = endpoint.replace(/\/+$/, "");
    return normalized === local.endpoint.replace(/\/+$/, "") ? local.localToken : undefined;
  };
  const container: Container = {
    client: () => {
      assertOpen();
      const baseUrl = currentRuntimeEndpoint();
      const localToken = localTokenFor(baseUrl);
      const signature = `${baseUrl}\u0000${localToken ?? ""}`;
      const storage = installedRuntimeMutationJournalStorage();
      if (shared?.signature === signature && shared.storage === storage) return shared.client;
      if (shared) retire(shared.client);
      const client = createLyraClient(createHttpTransport({ baseUrl, localToken }), {
        requestMeta: runtimeRequestMeta,
        capabilities: negotiatedCapabilities,
        mutationJournal: storage
          ? createMutationJournal({
              storage,
              scope: () => {
                const idempotency = negotiatedCapabilities()?.limits.idempotency;
                return idempotency
                  ? {
                      endpoint: baseUrl,
                      namespace: idempotency.namespace,
                      retentionSeconds: idempotency.retentionSeconds,
                    }
                  : null;
              },
            })
          : undefined,
      });
      shared = { signature, storage, client };
      return client;
    },
    sidecar: () => {
      assertOpen();
      const endpoint = currentRuntimeEndpoint();
      if (sidecar?.endpoint === endpoint) return sidecar.client;
      const client = createSidecarClient({ baseUrl: endpoint });
      sidecar = { endpoint, client };
      return client;
    },
    desktop: createDesktopHostClient(),
  };
  return {
    container,
    async initializeDesktopHost(desktop) {
      assertOpen();
      const generation = ++bootstrapGeneration;
      desktopBootstrap = null;
      const bootstrap = await desktop.bootstrap();
      if (closed || generation !== bootstrapGeneration) return;
      desktopBootstrap = bootstrap;
    },
    replaceDesktopHost() {
      assertOpen();
      bootstrapGeneration += 1;
      desktopBootstrap = null;
    },
    dispose() {
      if (disposal) return disposal;
      closed = true;
      bootstrapGeneration += 1;
      desktopBootstrap = null;
      if (shared) retire(shared.client);
      shared = null;
      sidecar = null;
      disposal = Promise.all(retiring).then(() => undefined);
      return disposal;
    },
  };
}

let defaultOwner = defaultContainer();
let instance: Container = defaultOwner.container;

export function getContainer(): Container {
  return instance;
}

/** Test seam — swap any subset of gateways with fakes. Other slots stay
 *  on the current defaults. */
export function setContainer(next: Partial<Container>): void {
  if (next.desktop) defaultOwner.replaceDesktopHost();
  instance = { ...instance, ...next };
}

/** Load app-owned bootstrap data before any plugin can construct an RPC client. */
export async function initializeDesktopHost(): Promise<void> {
  const owner = defaultOwner;
  const desktop = instance.desktop;
  await owner.initializeDesktopHost(desktop);
}

/** Begin final application teardown. The composition root only closes the
 * client it created; gateways injected by an embedding test remain externally
 * owned. Calling this from beforeunload is useful even though browsers do not
 * await it: LyraClient releases its journal lease synchronously before joining
 * the HTTP receive loop. */
export function disposeContainer(): Promise<void> {
  return defaultOwner.dispose();
}

/** Test seam — restore every gateway to its default wiring. Call from
 *  `afterEach` so one test's stubs don't bleed into the next. */
export async function resetContainer(): Promise<void> {
  const retired = defaultOwner;
  defaultOwner = defaultContainer();
  instance = defaultOwner.container;
  await retired.dispose();
}
