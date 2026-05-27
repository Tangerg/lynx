// Composition root — wires infra to domain gateways at app start.
// Singleton instead of Context because non-component code (zustand
// effects, plugin setup) calls these too; tests inject fakes via
// `setContainer()`.

import type { PermissionGateway } from "@/domain";
import { HttpPermissionGateway } from "@/infra/http/HttpPermissionGateway";
import { AGUI_BASE } from "@/main/config";
import type { Methods, RpcClient, SidecarClient } from "@/rpc";
import { createHttpTransport, createMethods, createRpcClient, createSidecarClient } from "@/rpc";

export interface Container {
  permission: PermissionGateway;
  rpc: RpcClient;
  methods: Methods;
  sidecar: SidecarClient;
}

function defaultContainer(): Container {
  // Default to the same HTTP loopback the legacy REST surface uses.
  // The HTTP transport / sidecar are still scaffolded — no callers
  // depend on them until the backend ships the new protocol. Existing
  // REST-based code paths keep working untouched.
  const transport = createHttpTransport({ baseUrl: AGUI_BASE });
  const rpc = createRpcClient(transport);
  return {
    permission: new HttpPermissionGateway(AGUI_BASE),
    rpc,
    methods: createMethods(rpc),
    sidecar: createSidecarClient({ baseUrl: AGUI_BASE }),
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
