// Host config is in-memory; mirror the runtime URL through plugin storage so the
// first RPC client built after launch sees the persisted endpoint.
//
// An adapter, not a use case: it takes the Host, names the storage slot the value
// lives in, and subscribes to config changes. It sat in `application/` next to the
// endpoint use cases — which is how a wiring function ends up in the ring that is
// supposed to be exercisable without any wiring.
//
// Not an `install*`: that prefix means "installs a port, here is its disposer" in
// this tree, and `host.config.onChange` is tracked by the Host and disposed on
// unload, so there is nothing to hand back.

import { RUNTIME_ENDPOINT_CONFIG_KEY } from "@/main/config";
import type { Host } from "@/plugins/sdk";

const ENDPOINT_STORAGE_KEY = "endpoint";

export function mirrorRuntimeEndpoint(host: Pick<Host, "config" | "storage">): void {
  const stored = host.storage.get<string>(ENDPOINT_STORAGE_KEY);
  if (typeof stored === "string" && stored) {
    host.config.set(RUNTIME_ENDPOINT_CONFIG_KEY, stored);
  }

  host.config.onChange(RUNTIME_ENDPOINT_CONFIG_KEY, (value) => {
    if (typeof value === "string") host.storage.set(ENDPOINT_STORAGE_KEY, value);
  });
}
