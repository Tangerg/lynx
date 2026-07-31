import type { ServerCapabilities } from "@/rpc";

/** Consumer-owned gateway; adapters remove the Runtime Protocol envelope. */
export interface RuntimeDiscovery {
  discoverCapabilities(): Promise<ServerCapabilities>;
}

const inFlight = new WeakMap<RuntimeDiscovery, Promise<ServerCapabilities>>();

export function discoverRuntime(discovery: RuntimeDiscovery): Promise<ServerCapabilities> {
  const existing = inFlight.get(discovery);
  if (existing) return existing;

  const current = Promise.resolve()
    .then(() => discovery.discoverCapabilities())
    .then((capabilities) => capabilities);
  inFlight.set(discovery, current);
  const clear = () => {
    if (inFlight.get(discovery) === current) inFlight.delete(discovery);
  };
  void current.then(clear, clear);
  return current;
}
