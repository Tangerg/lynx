import { getContainer } from "@/main/container";
import { discoverRuntime } from "@/main/runtimeProtocol";

export async function performRuntimeDiscovery(): Promise<void> {
  const { client } = getContainer();
  await discoverRuntime(client().rpc);
}
