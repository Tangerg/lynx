// Composition root — wires infra to domain gateways at app start.
// Singleton instead of Context because non-component code (zustand
// effects, plugin setup) calls these too; tests inject fakes via
// `setContainer()`.

import type { PermissionGateway } from "@/domain";
import { HttpPermissionGateway } from "@/infra/http/HttpPermissionGateway";
import { AGUI_BASE } from "@/main/config";
import { getConfig } from "@/plugins/sdk/config";
import type { Methods, RpcClient, SidecarClient } from "@/rpc";
import { createHttpTransport, createMethods, createRpcClient, createSidecarClient } from "@/rpc";

export interface Container {
  permission: PermissionGateway;
  /**
   * Lazy factory for the Lyra Runtime Protocol client (JSON-RPC over
   * HTTP). Not pre-instantiated because constructing the client
   * immediately opens an SSE connection to `/v1/rpc/stream` — the
   * current mock backend doesn't serve that yet, so we defer
   * construction until a caller actually wants it. Cutover PR (queries
   * + permission + AG-UI migration) will call this once and cache the
   * result.
   */
  createRpc: () => RpcClient;
  /** Typed method wrappers — bound to the RpcClient passed in. */
  createMethods: (rpc: RpcClient) => Methods;
  /**
   * Sidecar HTTP probe — pre-instantiated because it doesn't open a
   * persistent connection (each call is a one-shot fetch). Safe to
   * call against a backend that doesn't implement it yet (caller
   * handles the RpcTransportError).
   */
  sidecar: SidecarClient;
}

function defaultContainer(): Container {
  const baseUrl = AGUI_BASE;
  return {
    permission: new HttpPermissionGateway(baseUrl),
    createRpc: () =>
      // Read `api.localToken` at factory-call time so plugins (e.g. a
      // Wails-side bootstrap reading `~/.lyra/local-token`) can set it
      // via `host.config.set("api.localToken", ...)` before the first
      // RpcClient is instantiated. Web frontend hitting a same-machine
      // lyra-server needs this for the local process gate (docs/API.md
      // §1.2). For dev mock backend that doesn't validate, leave unset.
      createRpcClient(
        createHttpTransport({
          baseUrl,
          localToken: getConfig<string>("api.localToken") ?? undefined,
        }),
      ),
    createMethods,
    sidecar: createSidecarClient({ baseUrl }),
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
