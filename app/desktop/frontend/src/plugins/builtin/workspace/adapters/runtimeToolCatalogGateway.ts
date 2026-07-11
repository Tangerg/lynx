import { getContainer } from "@/main/container";
import { configureToolCatalogGateway } from "../application/ports/toolCatalogGateway";
import type { ToolCatalogGateway } from "../application/ports/toolCatalogGateway";

const gateway: ToolCatalogGateway = {
  async reconnectMCPServer(server) {
    await getContainer().client().workspace.mcp.reconnect(server);
  },
};

export function installToolCatalogGateway(): () => void {
  return configureToolCatalogGateway(gateway);
}
