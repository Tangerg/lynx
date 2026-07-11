import type { DiscoverResponse, RpcClient, ServerCapabilities } from "@/rpc";

export interface RuntimeCapabilitySink {
  replace(capabilities: ServerCapabilities): void;
}

const inFlight = new WeakMap<RpcClient, Promise<void>>();

export function discoverRuntime(
  rpc: RpcClient,
  capabilities: RuntimeCapabilitySink,
): Promise<void> {
  const existing = inFlight.get(rpc);
  if (existing) return existing;

  const current = Promise.resolve()
    .then(() => rpc.call<DiscoverResponse>("runtime.discover", {}))
    .then((result) => {
      capabilities.replace(result.capabilities);
    });
  inFlight.set(rpc, current);
  const clear = () => {
    if (inFlight.get(rpc) === current) inFlight.delete(rpc);
  };
  void current.then(clear, clear);
  return current;
}
