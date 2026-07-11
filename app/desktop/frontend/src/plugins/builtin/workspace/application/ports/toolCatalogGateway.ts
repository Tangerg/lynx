import { createSingletonPort } from "@/lib/ports/singletonPort";
export interface ToolCatalogGateway {
  reconnectMCPServer(server: string): Promise<void>;
}

const port = createSingletonPort<ToolCatalogGateway>("Tool catalog gateway is not configured");

export const configureToolCatalogGateway = port.configure;
export const toolCatalogGateway = port.get;
