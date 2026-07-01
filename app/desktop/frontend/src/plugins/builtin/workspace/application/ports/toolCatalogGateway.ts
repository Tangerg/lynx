export interface ToolCatalogGateway {
  reconnectMCPServer(server: string): Promise<void>;
}

let port: ToolCatalogGateway | null = null;

export function configureToolCatalogGateway(next: ToolCatalogGateway): void {
  port = next;
}

export function toolCatalogGateway(): ToolCatalogGateway {
  if (!port) throw new Error("Tool catalog gateway is not configured");
  return port;
}
