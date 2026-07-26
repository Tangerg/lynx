import { getContainer } from "@/main/container";
import type { RpcClient } from "@/rpc";

/** The live JSON-RPC client, for the one use case that speaks to it directly
 *  (capability discovery). Reaching the composition root is an adapter's job. */
export function runtimeRpc(): RpcClient {
  return getContainer().client().rpc;
}
