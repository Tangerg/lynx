// Composition root — wires infra to domain gateways at app start.
// Singleton instead of Context because non-component code (zustand
// effects, plugin setup) calls these too; tests inject fakes via
// `setContainer()`.

import type { PermissionGateway } from "@/domain";
import { HttpPermissionGateway } from "@/infra/http/HttpPermissionGateway";
import { AGUI_BASE } from "@/lib/http";

export interface Container {
  permission: PermissionGateway;
}

function defaultContainer(): Container {
  return {
    permission: new HttpPermissionGateway(AGUI_BASE),
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
