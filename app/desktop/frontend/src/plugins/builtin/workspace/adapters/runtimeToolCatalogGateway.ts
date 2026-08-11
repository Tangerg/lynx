import { getContainer } from "@/main/container";
import { configureToolCatalogGateway } from "../application/ports/toolCatalogGateway";
import type { ToolCatalogGateway } from "../application/ports/toolCatalogGateway";

const gateway: ToolCatalogGateway = {
  async reconnectMCPServer(server) {
    await getContainer().client().mcp.reconnect(server);
  },
  invokeDiagnosticTool(input) {
    return getContainer()
      .client()
      .tools.invoke({
        name: input.name,
        arguments: input.arguments,
        ...(input.cwd ? { workspace: { path: input.cwd } } : {}),
      });
  },
};

export function installToolCatalogGateway(): () => void {
  return configureToolCatalogGateway(gateway);
}
