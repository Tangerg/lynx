import {
  LyraClient,
  protocolVersion,
  type DiscoverResponse,
  type RuntimeConnection,
} from "@lyra/runtime-contract";

export async function discoverRuntime(
  connection: RuntimeConnection,
  signal?: AbortSignal,
): Promise<DiscoverResponse> {
  const response = await new LyraClient(connection).discover(
    {
      protocolVersion,
      clientInfo: { name: "lyra-desktop-app2", version: "0.0.0" },
      clientCapabilities: { features: {}, interruptTypes: [] },
    },
    signal,
  );
  if (
    response.protocolVersion !== connection.protocolVersion ||
    response.serverInfo.instanceId !== connection.instanceId ||
    response.capabilities.limits.idempotency.namespace !==
      connection.idempotencyNamespace
  ) {
    throw new Error("Runtime identity changed during discovery");
  }
  return response;
}
