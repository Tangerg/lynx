import { describeProblem } from "@/lib/rpcErrors";
import type { MCPServer } from "@/rpc";
import { mcpServerIcon, type MCPServerSettings } from "../application/mcpServerQueries";

export function mcpServerSettings(server: MCPServer): MCPServerSettings {
  const connection = server.connection;
  const status = server.status;
  return {
    id: server.name,
    name: server.name,
    desc: server.description ?? "",
    tools: status.type === "connected" ? status.toolCount : 0,
    status: status.type,
    errorDetail: "error" in status ? describeProblem(status.error) : undefined,
    icon: mcpServerIcon(server.name),
    type: connection.type,
    enabled: status.type !== "disabled",
    description: server.description,
    url: connection.type === "streamableHttp" ? connection.url : undefined,
    authorizationMasked:
      connection.type === "streamableHttp" ? connection.authorizationMasked : undefined,
    headersMasked: connection.type === "streamableHttp" ? connection.headersMasked : undefined,
    command: connection.type === "stdio" ? connection.command : undefined,
    args: connection.type === "stdio" ? connection.args : undefined,
    envMasked: connection.type === "stdio" ? connection.envMasked : undefined,
    dir: connection.type === "stdio" ? connection.dir : undefined,
    timeoutSeconds: server.timeoutSeconds,
    disabledTools: server.disabledTools,
    autoApproveTools: server.autoApproveTools,
    toolCount: status.type === "connected" ? status.toolCount : undefined,
  };
}
