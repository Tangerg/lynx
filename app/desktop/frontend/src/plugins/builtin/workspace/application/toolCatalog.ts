import type { MCPServer } from "@/lib/data/queries";
import { useBuiltinTools, useMCPServers, useMCPTools } from "@/lib/data/queries";
import { toolCatalogGateway } from "./ports/toolCatalogGateway";

export type MCPServerConfig = MCPServer;

export function useBuiltinToolConfigs() {
  return useBuiltinTools();
}

export function useMCPServerConfigs() {
  return useMCPServers();
}

export function useMCPServerToolConfigs(server: string) {
  return useMCPTools({ server });
}

export function reconnectMCPServer(server: string): void {
  toolCatalogGateway()
    .reconnectMCPServer(server)
    .catch((err: unknown) => console.warn("[mcp] reconnect failed:", err));
}
