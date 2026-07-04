import { getContainer } from "@/main/container";
import { performHandshake } from "@/main/handshake";

export async function performRuntimeHandshake(): Promise<void> {
  const { sidecar, client } = getContainer();
  await sidecar.info().catch(() => undefined);
  await performHandshake(client().rpc);
}
