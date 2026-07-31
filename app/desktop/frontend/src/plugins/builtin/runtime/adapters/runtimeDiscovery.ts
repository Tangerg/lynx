import { getContainer } from "@/main/container";
import type { RuntimeDiscovery } from "../application/discoverRuntime";

/** Translate typed Runtime Protocol discovery into the application's need. */
export function runtimeDiscovery(): RuntimeDiscovery {
  const client = getContainer().client();
  return {
    discoverCapabilities: async () => (await client.runtime.discover()).capabilities,
  };
}
