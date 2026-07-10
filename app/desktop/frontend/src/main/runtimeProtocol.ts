import { CLIENT_INFO, PROTOCOL_VERSION } from "@/main/config";
import type { ClientCapabilities, DiscoverResponse, RequestMeta, RpcClient } from "@/rpc";
import { useRuntimeStore } from "@/state/runtimeStore";

export const CLIENT_CAPABILITIES: ClientCapabilities = {
  events: [
    "run.started",
    "run.progress",
    "run.finished",
    "item.started",
    "item.delta",
    "item.completed",
    "state.snapshot",
    "state.delta",
    "custom",
  ],
  features: { multimodal: true },
  interruptTypes: ["approval", "question"],
};

export function runtimeRequestMeta(): RequestMeta {
  return {
    protocolVersion: PROTOCOL_VERSION,
    clientInfo: CLIENT_INFO,
    clientCapabilities: CLIENT_CAPABILITIES,
  };
}

const inFlight = new WeakMap<RpcClient, Promise<void>>();

export function discoverRuntime(rpc: RpcClient): Promise<void> {
  const existing = inFlight.get(rpc);
  if (existing) return existing;

  const current = Promise.resolve()
    .then(() => rpc.call<DiscoverResponse>("runtime.discover", {}))
    .then((result) => {
      useRuntimeStore.getState().setDiscovery(result.capabilities);
    });
  inFlight.set(rpc, current);
  const clear = () => {
    if (inFlight.get(rpc) === current) inFlight.delete(rpc);
  };
  void current.then(clear, clear);
  return current;
}
