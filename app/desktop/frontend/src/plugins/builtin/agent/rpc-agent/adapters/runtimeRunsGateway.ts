import { getContainer } from "@/main/container";
import { asSessionId } from "@/rpc";
import type { RpcRunsGateway } from "../application/rpcAgentDriver";

/**
 * The runs gateway the RPC agent source drives, bound to the live client.
 *
 * This lived as an object literal inside the plugin's `setup()`, which put the
 * two things a root is not supposed to hold — reaching the composition root, and
 * coercing the app's session id into the wire's branded one — in the file whose
 * whole job is assembly. Every sibling context keeps that pair in `adapters/`;
 * the rule was written down for `defaults/` only, so it read as a local
 * exception rather than the shape it is.
 */
export function runtimeRunsGateway(): RpcRunsGateway {
  return {
    start: ({ sessionId, ...params }, signal) =>
      getContainer()
        .client()
        .runs.start({ ...params, sessionId: asSessionId(sessionId) }, signal),
    resume: (params, signal) => getContainer().client().runs.resume(params, signal),
  };
}
