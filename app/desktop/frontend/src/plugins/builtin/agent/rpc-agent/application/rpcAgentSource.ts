import type { AgentSourceSpec } from "@/plugins/sdk";
import { activeRpcSessionId, createRpcAgentDriver, type RpcRunsGateway } from "./rpcAgentDriver";

export type Translate = (key: string) => string;
export type ActiveSessionId = () => string | null | undefined;

export function rpcAgentSource(
  t: Translate,
  activeSessionId: ActiveSessionId,
  gateway: () => RpcRunsGateway,
): AgentSourceSpec {
  return {
    id: "rpc",
    label: t("agentSource.rpc"),
    priority: 1,
    factory: () => createRpcAgentDriver(activeRpcSessionId(activeSessionId()), gateway),
  };
}
