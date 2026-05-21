// Composition root — wires concrete infra implementations to the
// domain gateways at app start, then exposes a single accessor
// (`getContainer()`) for everything else to read from.
//
// Why a singleton object, not React Context: most gateways are called
// from non-component code too (zustand effects, plugin setup, fetch
// retries inside hooks). A Context would force everything to thread
// through a hook, which doesn't fit. The Context-wrapper exists in
// proper clean-react-app setups so unit tests can inject fakes; for
// Lyra we just expose `setContainer()` for the same purpose.
//
// Adding a new gateway:
//   1. Declare interface in `@/domain/gateways/*`
//   2. Implement in `@/infra/...`
//   3. Add field here + bootstrap line in `defaultContainer()`
//   4. UI calls `getContainer().yourGateway.method(...)`

import type { PermissionGateway } from "@/domain";
import { HttpPermissionGateway } from "@/infra/http/HttpPermissionGateway";
import { AGUI_BASE } from "@/lib/http";

export type Container = {
  permission: PermissionGateway;
};

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
